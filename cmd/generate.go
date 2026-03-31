package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/phlisg/frank-cli/internal/compose"
	"github.com/phlisg/frank-cli/internal/config"
	"github.com/phlisg/frank-cli/internal/template"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(generateCmd)
}

var generateCmd = &cobra.Command{
	Use:          "generate",
	Short:        "Generate Docker files from frank.yaml",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := resolveDir()
		cfg, err := config.Load(dir)
		if err != nil {
			return err
		}
		return generate(cfg, dir)
	},
}

// generate runs the full file generation pipeline for cfg into dir.
// Called by both `frank generate` and at the end of `frank init`.
func generate(cfg *config.Config, dir string) error {
	projectName := config.ProjectName(dir)
	engine := template.New(TemplateFS)
	gen := compose.New(engine)

	if err := gen.Write(cfg, projectName, dir); err != nil {
		return fmt.Errorf("generate compose.yaml: %w", err)
	}
	fmt.Println("  wrote  compose.yaml")

	if err := gen.WriteEnv(cfg, projectName, dir); err != nil {
		return fmt.Errorf("generate .env: %w", err)
	}
	fmt.Println("  wrote  .env")
	fmt.Println("  wrote  .env.example")

	data := template.Data{
		PHPVersion:  cfg.PHP.Version,
		ProjectName: projectName,
	}

	dockerfile, err := engine.RenderRuntime(cfg.PHP.Runtime, "Dockerfile.tmpl", data)
	if err != nil {
		return fmt.Errorf("render Dockerfile: %w", err)
	}
	if err := writeFile(filepath.Join(dir, "Dockerfile"), dockerfile); err != nil {
		return err
	}
	fmt.Println("  wrote  Dockerfile")

	switch cfg.PHP.Runtime {
	case "frankenphp":
		caddyfile, err := engine.RenderRuntime("frankenphp", "Caddyfile.tmpl", data)
		if err != nil {
			return fmt.Errorf("render Caddyfile: %w", err)
		}
		if err := writeFile(filepath.Join(dir, "Caddyfile"), caddyfile); err != nil {
			return err
		}
		fmt.Println("  wrote  Caddyfile")

	case "fpm":
		nginxConf, err := engine.RenderRuntime("fpm", "nginx.conf.tmpl", data)
		if err != nil {
			return fmt.Errorf("render nginx.conf: %w", err)
		}
		if err := writeFile(filepath.Join(dir, "nginx.conf"), nginxConf); err != nil {
			return err
		}
		fmt.Println("  wrote  nginx.conf")

		nginxDockerfile, err := engine.RenderRuntime("fpm", "nginx.Dockerfile.tmpl", data)
		if err != nil {
			return fmt.Errorf("render nginx.Dockerfile: %w", err)
		}
		if err := writeFile(filepath.Join(dir, "nginx.Dockerfile"), nginxDockerfile); err != nil {
			return err
		}
		fmt.Println("  wrote  nginx.Dockerfile")
	}

	return nil
}
