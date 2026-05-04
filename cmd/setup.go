package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/huh"
	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/docker"
	"github.com/phlisg/frank/internal/output"
	"github.com/phlisg/frank/internal/tool"
	"github.com/spf13/cobra"
)

func init() {
	setupCmd.Flags().BoolVar(&sailMode, "sail", false, "generate a Sail-compatible project (no Frank traces)")
	rootCmd.AddCommand(setupCmd)
}

var setupCmd = &cobra.Command{
	Use:               "setup",
	Short:             "Configure Frank in an existing Laravel project",
	SilenceUsage:      true,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE:              runSetup,
}

func runSetup(cmd *cobra.Command, args []string) error {
	dir, err := filepath.Abs(resolveDir())
	if err != nil {
		return err
	}

	// Require existing Laravel project.
	if _, err := os.Stat(filepath.Join(dir, "artisan")); os.IsNotExist(err) {
		return fmt.Errorf("not a Laravel project — use `frank new` to create one")
	}

	if sailMode {
		return runSetupSail(dir)
	}

	return runSetupFrank(cmd, dir)
}

func runSetupFrank(cmd *cobra.Command, dir string) error {
	cfg := config.New()

	// Pre-populate from existing frank.yaml if present.
	existing, loadErr := config.Load(dir)
	if loadErr == nil {
		cfg = existing
	}

	selectedServices := cfg.Services
	if len(selectedServices) == 0 {
		selectedServices = []string{"pgsql", "mailpit"}
	}

	scheduleWorker := cfg.Workers.Schedule
	queueCount := 1
	if len(cfg.Workers.Queue) > 0 {
		queueCount = cfg.Workers.Queue[0].Count
	}

	var groups []*huh.Group

	groups = append(groups, huh.NewGroup(
		huh.NewSelect[string]().
			Title("PHP Version").
			Options(
				huh.NewOption("8.5 (latest)", "8.5"),
				huh.NewOption("8.4", "8.4"),
				huh.NewOption("8.3", "8.3"),
				huh.NewOption("8.2", "8.2"),
			).
			Value(&cfg.PHP.Version),
	))

	groups = append(groups, huh.NewGroup(
		huh.NewSelect[string]().
			Title("Runtime").
			Description("FrankenPHP is an all-in-one server; FPM uses a separate Nginx container.").
			Options(
				huh.NewOption("FrankenPHP (recommended)", "frankenphp"),
				huh.NewOption("PHP-FPM + Nginx", "fpm"),
			).
			Value(&cfg.PHP.Runtime),
	))

	groups = append(groups, huh.NewGroup(
		huh.NewSelect[string]().
			Title("Package Manager").
			Options(
				huh.NewOption("npm (default)", "npm"),
				huh.NewOption("pnpm", "pnpm"),
				huh.NewOption("bun", "bun"),
			).
			Value(&cfg.Node.PackageManager),
	))

	groups = append(groups, huh.NewGroup(
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
	))

	groups = append(groups, huh.NewGroup(
		huh.NewConfirm().
			Title("Schedule worker").
			Description("Run php artisan schedule:work in a dedicated container?").
			Affirmative("Yes").
			Negative("No").
			Value(&scheduleWorker),
	))

	groups = append(groups, huh.NewGroup(
		huh.NewSelect[int]().
			Title("Queue workers").
			Description("How many php artisan queue:work containers to run on the default queue?").
			Options(
				huh.NewOption("None", 0),
				huh.NewOption("1", 1),
				huh.NewOption("2", 2),
				huh.NewOption("3", 3),
				huh.NewOption("4", 4),
			).
			Value(&queueCount),
	))

	if err := huh.NewForm(groups...).Run(); err != nil {
		return err
	}

	cfg.Services = selectedServices
	applyWorkersFromInit(cfg, scheduleWorker, queueCount)

	// Tool selection — always interactive for setup.
	allTools := tool.AllNames()
	selectedTools := make([]string, len(allTools))
	copy(selectedTools, allTools)

	// Pre-populate from existing config if available.
	if loadErr == nil && len(cfg.Tools) > 0 {
		selectedTools = cfg.Tools
	}

	options := make([]huh.Option[string], len(allTools))
	for i, t := range allTools {
		options[i] = huh.NewOption(t, t)
	}
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Dev tools").
				Options(options...).
				Value(&selectedTools),
		),
	).Run(); err != nil {
		return err
	}
	cfg.Tools = selectedTools

	// Write frank.yaml + generate .frank/ + install tools (no Laravel install).
	if err := setupWriteAndGenerate(cfg, dir); err != nil {
		return err
	}

	// Prompt to rebuild containers.
	if err := setupRebuildPrompt(dir); err != nil {
		return err
	}

	if cfg.Server.IsHTTPS() {
		printViteHTTPSHint(dir)
	}
	output.NextSteps([]string{"frank up -d"})
	return nil
}

func runSetupSail(dir string) error {
	cfg := config.New()
	cfg.PHP.Runtime = "fpm"

	// Pre-populate from existing frank.yaml if present.
	existing, loadErr := config.Load(dir)
	if loadErr == nil {
		cfg = existing
		cfg.PHP.Runtime = "fpm" // Sail always FPM.
	}

	selectedServices := []string{"mysql", "mailpit"}
	if loadErr == nil && len(cfg.Services) > 0 {
		selectedServices = cfg.Services
	}

	var groups []*huh.Group

	groups = append(groups, huh.NewGroup(
		huh.NewSelect[string]().
			Title("PHP Version").
			Options(
				huh.NewOption("8.5 (latest)", "8.5"),
				huh.NewOption("8.4", "8.4"),
				huh.NewOption("8.3", "8.3"),
				huh.NewOption("8.2", "8.2"),
			).
			Value(&cfg.PHP.Version),
	))

	groups = append(groups, huh.NewGroup(
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
	))

	if err := huh.NewForm(groups...).Run(); err != nil {
		return err
	}

	cfg.Services = selectedServices

	// Sail install — no Frank file generation, just sail:install.
	var sailServices []string
	for _, svc := range cfg.Services {
		if svc == "sqlite" {
			continue
		}
		sailServices = append(sailServices, svc)
	}
	if err := runSailInstall(dir, sailServices, cfg.PHP.Version); err != nil {
		return fmt.Errorf("sail install: %w", err)
	}

	output.NextSteps([]string{"vendor/bin/sail up"})
	return nil
}

// setupWriteAndGenerate writes frank.yaml, generates .frank/ files, and installs
// dev tools. Unlike writeConfigAndGenerate in init.go, this does NOT install
// Laravel or run composerRequireDev — the project already exists.
func setupWriteAndGenerate(cfg *config.Config, dir string) error {
	existingCompose := detectExistingCompose(dir)
	if existingCompose != "" {
		output.Detail(fmt.Sprintf("note: %s will be replaced by generated compose.yaml", existingCompose))
	}

	yamlBytes, err := marshalConfig(cfg)
	if err != nil {
		return err
	}
	if err := writeFile(filepath.Join(dir, config.ConfigFileName), yamlBytes); err != nil {
		return err
	}
	output.Detail("wrote frank.yaml")
	output.Group("Wrote frank.yaml", "")

	stopGen := output.Spin("Generating Docker files")
	if err := generate(cfg, dir, rootCmd.Version); err != nil {
		stopGen(err)
		return err
	}
	stopGen(nil)

	if len(cfg.Tools) > 0 {
		stopTools := output.Spin("Installing dev tools")
		res, err := tool.Install(cfg.Tools, dir)
		if err != nil {
			stopTools(err)
			return err
		}
		stopTools(nil)
		output.Detail(fmt.Sprintf("%d created, %d skipped", len(res.Created), len(res.Skipped)))
	}

	return nil
}

// setupRebuildPrompt asks the user whether to rebuild and restart containers.
func setupRebuildPrompt(dir string) error {
	dc := docker.New(dir)
	state, _, _ := dc.ContainerStatus()
	running := state == docker.StateRunning

	title := "Rebuild containers now?"
	if running {
		title = "Rebuild and restart containers now?"
	}

	var rebuild bool
	if err := huh.NewConfirm().
		Title(title).
		Value(&rebuild).
		Run(); err != nil {
		return err
	}

	if !rebuild {
		return nil
	}

	if running {
		if output.GetLevel() == output.Verbose {
			return dc.Run("up", "-d", "--build")
		}
		_, err := dc.RunQuiet("up", "-d", "--build")
		return err
	}

	if output.GetLevel() == output.Verbose {
		return dc.Run("build")
	}
	_, err := dc.RunQuiet("build")
	return err
}
