package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/phlisg/frank/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var sailMode bool

func init() {
	initCmd.Flags().BoolVar(&sailMode, "sail", false, "generate a Sail-compatible project (no Frank traces)")
	rootCmd.AddCommand(initCmd)
}

var initCmd = &cobra.Command{
	Use:          "init",
	Short:        "Interactive wizard to create frank.yaml",
	SilenceUsage: true,
	RunE:         runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	dir := resolveDir()

	// If --dir was explicitly set and the directory doesn't exist, offer to create it.
	if Dir != "" {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			var create bool
			prompt := huh.NewForm(
				huh.NewGroup(
					huh.NewConfirm().
						Title(fmt.Sprintf("Directory %q does not exist. Create it?", dir)).
						Value(&create),
				),
			)
			if err := prompt.Run(); err != nil {
				return err
			}
			if !create {
				return fmt.Errorf("directory %q does not exist", dir)
			}
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("create directory: %w", err)
			}
		}
	}

	existingCompose := detectExistingCompose(dir)

	cfg := config.New()

	if sailMode {
		return runSailInit(cfg, dir, existingCompose)
	}

	return runFrankInit(cfg, dir, existingCompose)
}

func runFrankInit(cfg *config.Config, dir, existingCompose string) error {
	selectedServices := []string{"pgsql", "mailpit"}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("PHP Version").
				Options(
					huh.NewOption("8.5 (latest)", "8.5"),
					huh.NewOption("8.4", "8.4"),
					huh.NewOption("8.3", "8.3"),
					huh.NewOption("8.2", "8.2"),
				).
				Value(&cfg.PHP.Version),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Laravel Version").
				Options(
					huh.NewOption("13.x (latest)", "13.*"),
					huh.NewOption("12.x (LTS)", "12.*"),
					huh.NewOption("11.x", "11.*"),
				).
				Value(&cfg.Laravel.Version),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Runtime").
				Description("FrankenPHP is an all-in-one server; FPM uses a separate Nginx container.").
				Options(
					huh.NewOption("FrankenPHP (recommended)", "frankenphp"),
					huh.NewOption("PHP-FPM + Nginx", "fpm"),
				).
				Value(&cfg.PHP.Runtime),
		),
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Services").
				Description("Select the services your project needs. Only one database may be chosen.").
				Options(
					huh.NewOption("PostgreSQL", "pgsql"),
					huh.NewOption("MySQL", "mysql"),
					huh.NewOption("MariaDB", "mariadb"),
					huh.NewOption("SQLite", "sqlite"),
					huh.NewOption("Redis", "redis"),
					huh.NewOption("Memcached", "memcached"),
					huh.NewOption("Meilisearch", "meilisearch"),
					huh.NewOption("Mailpit", "mailpit"),
				).
				Value(&selectedServices),
		),
	)

	if err := form.Run(); err != nil {
		return err
	}

	cfg.Services = selectedServices

	return writeConfigAndGenerate(cfg, dir, existingCompose)
}

func runSailInit(cfg *config.Config, dir, existingCompose string) error {
	// Sail uses FPM under the hood
	cfg.PHP.Runtime = "fpm"
	selectedServices := []string{"pgsql", "mailpit"}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("PHP Version").
				Options(
					huh.NewOption("8.5 (latest)", "8.5"),
					huh.NewOption("8.4", "8.4"),
					huh.NewOption("8.3", "8.3"),
					huh.NewOption("8.2", "8.2"),
				).
				Value(&cfg.PHP.Version),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Laravel Version").
				Options(
					huh.NewOption("12.x (latest)", "12.*"),
					huh.NewOption("11.x (LTS)", "11.*"),
					huh.NewOption("10.x", "10.*"),
				).
				Value(&cfg.Laravel.Version),
		),
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Services").
				Description("Which services would you like to install?").
				Options(
					huh.NewOption("pgsql", "pgsql"),
					huh.NewOption("mysql", "mysql"),
					huh.NewOption("mariadb", "mariadb"),
					huh.NewOption("redis", "redis"),
					huh.NewOption("memcached", "memcached"),
					huh.NewOption("meilisearch", "meilisearch"),
					huh.NewOption("mailpit", "mailpit"),
				).
				Value(&selectedServices),
		),
	)

	if err := form.Run(); err != nil {
		return err
	}

	cfg.Services = selectedServices

	return writeConfigAndGenerate(cfg, dir, existingCompose)
}

func writeConfigAndGenerate(cfg *config.Config, dir, existingCompose string) error {
	if existingCompose != "" {
		fmt.Printf("\nNote: %s will be replaced by the generated compose.yaml.\n", existingCompose)
	}

	yamlBytes, err := marshalConfig(cfg)
	if err != nil {
		return err
	}
	if err := writeFile(filepath.Join(dir, config.ConfigFileName), yamlBytes); err != nil {
		return err
	}
	fmt.Println("\n  wrote  frank.yaml")

	fmt.Println("\nGenerating Docker files...")
	return generate(cfg, dir)
}

func marshalConfig(cfg *config.Config) (string, error) {
	cfg.Version = 1

	// Build a clean ordered map so the YAML field order is predictable.
	type configOutput struct {
		Version  int                             `yaml:"version"`
		PHP      config.PHP                      `yaml:"php"`
		Laravel  config.Laravel                  `yaml:"laravel"`
		Services []string                        `yaml:"services"`
		Config   map[string]config.ServiceConfig `yaml:"config,omitempty"`
	}

	out := configOutput{
		Version:  cfg.Version,
		PHP:      cfg.PHP,
		Laravel:  cfg.Laravel,
		Services: cfg.Services,
		Config:   cfg.Config,
	}

	b, err := yaml.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("marshal frank.yaml: %w", err)
	}
	header := "# Generated by Frank — edit this file to customise your environment\n\n"
	return header + strings.TrimSpace(string(b)) + "\n", nil
}

// detectExistingCompose returns the name of any existing compose file in dir, or "".
func detectExistingCompose(dir string) string {
	for _, name := range []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return name
		}
	}
	return ""
}
