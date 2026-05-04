package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/docker"
	"github.com/phlisg/frank/internal/output"
	"github.com/phlisg/frank/internal/tool"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// Package-level flag vars shared with setup.go.
var sailMode bool
var flagPHP string
var flagLaravel string
var flagWith string
var flagRuntime string
var flagPM string
var flagSchedule bool
var flagQueueCount int
var flagNoPint bool
var flagNoLarastan bool
var flagNoRector bool
var flagNoLefthook bool
var flagNoTools bool

var flagNoUp bool
var flagHTTP bool
var flagInteractive bool

func init() {
	newCmd.Flags().BoolVar(&sailMode, "sail", false, "generate a Sail-compatible project (no Frank traces)")
	newCmd.Flags().StringVar(&flagPHP, "php", "", "PHP version (e.g. 8.5)")
	newCmd.Flags().StringVar(&flagLaravel, "laravel", "", "Laravel version (e.g. 13 or 13.*)")
	newCmd.Flags().StringVar(&flagWith, "with", "", `comma-separated services (e.g. "pgsql,redis,mailpit")`)
	newCmd.Flags().StringVar(&flagRuntime, "runtime", "", "runtime: frankenphp or fpm (ignored with --sail)")
	newCmd.Flags().StringVar(&flagPM, "pm", "", "package manager: npm, pnpm, bun")
	newCmd.Flags().BoolVar(&flagSchedule, "schedule", false, "enable schedule:work worker")
	newCmd.Flags().IntVar(&flagQueueCount, "queue-count", 0, "number of queue:work workers (0-4)")
	newCmd.Flags().BoolVar(&flagNoPint, "no-pint", false, "exclude pint from dev tools")
	newCmd.Flags().BoolVar(&flagNoLarastan, "no-larastan", false, "exclude larastan from dev tools")
	newCmd.Flags().BoolVar(&flagNoRector, "no-rector", false, "exclude rector from dev tools")
	newCmd.Flags().BoolVar(&flagNoLefthook, "no-lefthook", false, "exclude lefthook from dev tools")
	newCmd.Flags().BoolVar(&flagNoTools, "no-tools", false, "skip dev tools entirely")
	newCmd.Flags().BoolVar(&flagNoUp, "no-up", false, "skip container start after install")
	newCmd.Flags().BoolVar(&flagHTTP, "http", false, "disable HTTPS (serve over plain HTTP)")
	newCmd.Flags().BoolVar(&flagInteractive, "interactive", false, "run full interactive wizard")
	rootCmd.AddCommand(newCmd)
}

var newCmd = &cobra.Command{
	Use:   "new <project>",
	Short: "Create a new Laravel project (zero to localhost)",
	Long: `Create a new Laravel project with a complete Docker environment.
Runs the full pipeline: scaffold, install Laravel, generate Docker files,
start containers, and install npm dependencies.

Examples:
  frank new my-app                          # all defaults, starts containers
  frank new my-app --php 8.4 --with redis   # override via flags
  frank new my-app --no-up                  # skip container start
  frank new my-app --interactive            # full wizard
  frank new my-app --sail                   # sail-only install`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return nil, cobra.ShellCompDirectiveFilterDirs
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: runNew,
}

func runNew(cmd *cobra.Command, args []string) error {
	projectName := args[0]

	// 1. Pre-flight: docker dependencies
	if err := docker.CheckDependencies(); err != nil {
		return err
	}

	// 2. Create directory
	target := projectName
	if !filepath.IsAbs(target) {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory: %w", err)
		}
		target = filepath.Join(cwd, target)
	}

	if info, err := os.Stat(target); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("%q exists and is not a directory", projectName)
		}
		entries, err := os.ReadDir(target)
		if err != nil {
			return fmt.Errorf("read directory: %w", err)
		}
		if len(entries) > 0 {
			return fmt.Errorf("directory %q already exists and is not empty", projectName)
		}
	} else if os.IsNotExist(err) {
		if err := os.MkdirAll(target, 0755); err != nil {
			return fmt.Errorf("create directory: %w", err)
		}
	} else {
		return fmt.Errorf("stat directory: %w", err)
	}

	dir := target

	// --interactive delegates to the existing wizard
	if flagInteractive {
		// Temporarily set Dir so resolveDir() returns target.
		Dir = dir
		defer func() { Dir = "" }()

		existingCompose := detectExistingCompose(dir)
		cfg := config.New()

		var initErr error
		if sailMode {
			initErr = runSailInit(cfg, dir, existingCompose)
		} else {
			initErr = runFrankInit(cmd, cfg, dir, existingCompose)
		}
		if initErr != nil {
			output.Warning(fmt.Sprintf("failed: %v — remove the directory and start fresh", initErr))
			return initErr
		}

		if !flagNoUp {
			if err := runNewUp(dir, cfg); err != nil {
				return err
			}
		}

		printNewNextSteps(projectName, dir, !flagNoUp, cfg)
		return nil
	}

	// Non-interactive pipeline
	cfg := config.New()

	// Apply defaults for non-interactive mode
	if flagPHP != "" {
		cfg.PHP.Version = flagPHP
	}
	// config.New() sets "latest" — override to "13.*" unless --laravel was given
	if flagLaravel != "" {
		cfg.Laravel.Version = normalizeLaravelVersion(flagLaravel)
	} else {
		cfg.Laravel.Version = "13.*"
	}
	if flagRuntime != "" {
		cfg.PHP.Runtime = flagRuntime
	}
	if flagPM != "" {
		switch flagPM {
		case "npm", "pnpm", "bun":
			cfg.Node.PackageManager = flagPM
		default:
			return fmt.Errorf("invalid --pm %q: valid options are npm, pnpm, bun", flagPM)
		}
	}

	if flagHTTP {
		f := false
		cfg.Server.HTTPS = &f
	}

	// Services
	if sailMode {
		if flagWith != "" {
			cfg.Services = parseServices(flagWith)
		} else {
			cfg.Services = []string{"mysql", "mailpit"}
		}
	} else {
		if flagWith != "" {
			cfg.Services = parseServices(flagWith)
		} else {
			cfg.Services = []string{"pgsql", "mailpit"}
		}
	}

	// Workers (non-interactive defaults: schedule on, 1 queue worker)
	scheduleWorker := flagSchedule
	queueCount := flagQueueCount
	if !cmd.Flags().Changed("schedule") {
		scheduleWorker = true
	}
	if !cmd.Flags().Changed("queue-count") {
		queueCount = 1
	}
	applyWorkersFromInit(cfg, scheduleWorker, queueCount)

	// Tools
	if !flagNoTools {
		allTools := tool.AllNames()
		cfg.Tools = make([]string, 0, len(allTools))
		for _, t := range allTools {
			switch t {
			case "pint":
				if !flagNoPint {
					cfg.Tools = append(cfg.Tools, t)
				}
			case "larastan":
				if !flagNoLarastan {
					cfg.Tools = append(cfg.Tools, t)
				}
			case "rector":
				if !flagNoRector {
					cfg.Tools = append(cfg.Tools, t)
				}
			case "lefthook":
				if !flagNoLefthook {
					cfg.Tools = append(cfg.Tools, t)
				}
			default:
				cfg.Tools = append(cfg.Tools, t)
			}
		}
	}

	// Sail mode: set runtime to fpm, delegate to sail install path
	if sailMode {
		cfg.PHP.Runtime = "fpm"
		existingCompose := detectExistingCompose(dir)
		if err := writeConfigAndGenerate(cfg, dir, existingCompose); err != nil {
			output.Warning(fmt.Sprintf("failed: %v — remove the directory and start fresh", err))
			return err
		}

		var sailServices []string
		for _, svc := range cfg.Services {
			if svc == "sqlite" {
				continue
			}
			sailServices = append(sailServices, svc)
		}
		if err := runSailInstall(dir, sailServices, cfg.PHP.Version); err != nil {
			output.Warning(fmt.Sprintf("sail install failed: %v — remove the directory and start fresh", err))
			return fmt.Errorf("sail install: %w", err)
		}

		output.NextSteps([]string{
			fmt.Sprintf("cd %s", projectName),
			"vendor/bin/sail up",
		})
		return nil
	}

	// 3-7. Write config, generate, install Laravel, dev tools
	existingCompose := detectExistingCompose(dir)
	if err := writeConfigAndGenerate(cfg, dir, existingCompose); err != nil {
		output.Warning(fmt.Sprintf("failed: %v — remove the directory and start fresh", err))
		return err
	}

	// 8. Start containers + npm install
	if !flagNoUp {
		if err := runNewUp(dir, cfg); err != nil {
			return err
		}
	}

	// 9. NextSteps
	printNewNextSteps(projectName, dir, !flagNoUp, cfg)
	return nil
}

// runNewUp runs doUp then npm install inside the container.
func runNewUp(dir string, cfg *config.Config) error {
	if err := doUp(dir, true, false, nil, false); err != nil {
		output.Warning(fmt.Sprintf("containers failed to start: %v", err))
		return err
	}

	// npm install inside the container
	client := docker.New(dir)
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
		pm := "npm"
		if cfg != nil && cfg.Node.PackageManager != "" {
			pm = cfg.Node.PackageManager
		}

		stopNpm := output.Spin(fmt.Sprintf("Installing %s dependencies", pm))

		// Wait for container to be ready
		if err := client.WaitForContainer("laravel.test", 30*time.Second); err != nil {
			output.Warning(fmt.Sprintf("container not ready for %s install: %v", pm, err))
			stopNpm(nil)
			return nil
		}

		var npmErr error
		if output.GetLevel() == output.Verbose {
			npmErr = client.Exec("laravel.test", pm, "install")
		} else {
			_, npmErr = client.ExecQuiet("laravel.test", pm, "install")
		}
		if npmErr != nil {
			stopNpm(fmt.Errorf("%s install failed: %w", pm, npmErr))
		} else {
			stopNpm(nil)
		}
	}

	return nil
}

// printNewNextSteps prints the final NextSteps block for frank new.
func printNewNextSteps(projectName, dir string, containersStarted bool, cfg *config.Config) {
	if cfg.Server.IsHTTPS() {
		printViteHTTPSHint(dir)
	}
	steps := []string{fmt.Sprintf("cd %s", projectName)}
	if containersStarted {
		scheme := "http"
		if cfg.Server.IsHTTPS() {
			scheme = "https"
		}
		port := ""
		if cfg.Server.Port != 0 {
			port = fmt.Sprintf(":%d", cfg.Server.Port)
		}
		steps = append(steps, fmt.Sprintf("%s://localhost%s", scheme, port))
	} else {
		steps = append(steps, "frank up -d")
	}
	steps = append(steps, "")
	steps = append(steps, "Tip: run `frank activate` to load shell aliases (up, down, artisan, etc.)")
	output.NextSteps(steps)
}

// --- Interactive wizard (used by frank new --interactive) ---

func runFrankInit(cmd *cobra.Command, cfg *config.Config, dir, existingCompose string) error {
	selectedServices := []string{"pgsql", "mailpit"}
	scheduleWorker := flagSchedule
	queueCount := flagQueueCount

	if !cmd.Flags().Changed("schedule") {
		scheduleWorker = true
	}
	if !cmd.Flags().Changed("queue-count") {
		queueCount = 1
	}

	var groups []*huh.Group

	if flagPHP != "" {
		cfg.PHP.Version = flagPHP
	} else {
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
	}

	if flagLaravel != "" {
		cfg.Laravel.Version = normalizeLaravelVersion(flagLaravel)
	} else {
		groups = append(groups, huh.NewGroup(
			huh.NewSelect[string]().
				Title("Laravel Version").
				Options(
					huh.NewOption("13.x (latest)", "13.*"),
					huh.NewOption("12.x (LTS)", "12.*"),
				).
				Value(&cfg.Laravel.Version),
		))
	}

	if flagRuntime != "" {
		cfg.PHP.Runtime = flagRuntime
	} else {
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
	}

	if flagPM != "" {
		switch flagPM {
		case "npm", "pnpm", "bun":
			cfg.Node.PackageManager = flagPM
		default:
			return fmt.Errorf("invalid --pm %q: valid options are npm, pnpm, bun", flagPM)
		}
	} else {
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
	}

	if flagWith != "" {
		selectedServices = parseServices(flagWith)
	} else {
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
	}

	if !cmd.Flags().Changed("schedule") {
		groups = append(groups, huh.NewGroup(
			huh.NewConfirm().
				Title("Schedule worker").
				Description("Run php artisan schedule:work in a dedicated container?").
				Affirmative("Yes").
				Negative("No").
				Value(&scheduleWorker),
		))
	}

	if !cmd.Flags().Changed("queue-count") {
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
	}

	if err := huh.NewForm(groups...).Run(); err != nil {
		return err
	}

	cfg.Services = selectedServices
	applyWorkersFromInit(cfg, scheduleWorker, queueCount)

	if !flagNoTools {
		allTools := tool.AllNames()
		if flagPHP != "" || sailMode {
			cfg.Tools = make([]string, 0)
			for _, t := range allTools {
				switch t {
				case "pint":
					if !flagNoPint {
						cfg.Tools = append(cfg.Tools, t)
					}
				case "larastan":
					if !flagNoLarastan {
						cfg.Tools = append(cfg.Tools, t)
					}
				case "rector":
					if !flagNoRector {
						cfg.Tools = append(cfg.Tools, t)
					}
				case "lefthook":
					if !flagNoLefthook {
						cfg.Tools = append(cfg.Tools, t)
					}
				default:
					cfg.Tools = append(cfg.Tools, t)
				}
			}
		} else {
			selectedTools := make([]string, len(allTools))
			copy(selectedTools, allTools)
			options := make([]huh.Option[string], len(allTools))
			for i, t := range allTools {
				options[i] = huh.NewOption(t, t)
			}
			err := huh.NewForm(
				huh.NewGroup(
					huh.NewMultiSelect[string]().
						Title("Dev tools").
						Options(options...).
						Value(&selectedTools),
				),
			).Run()
			if err != nil {
				return err
			}
			cfg.Tools = selectedTools
		}
	}

	return writeConfigAndGenerate(cfg, dir, existingCompose)
}

func runSailInit(cfg *config.Config, dir, existingCompose string) error {
	cfg.PHP.Runtime = "fpm"
	selectedServices := []string{"pgsql", "mailpit"}

	var groups []*huh.Group

	if flagPHP != "" {
		cfg.PHP.Version = flagPHP
	} else {
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
	}

	if flagLaravel != "" {
		cfg.Laravel.Version = normalizeLaravelVersion(flagLaravel)
	} else {
		groups = append(groups, huh.NewGroup(
			huh.NewSelect[string]().
				Title("Laravel Version").
				Options(
					huh.NewOption("13.x (latest)", "13.*"),
					huh.NewOption("12.x (LTS)", "12.*"),
				).
				Value(&cfg.Laravel.Version),
		))
	}

	if flagWith != "" {
		selectedServices = parseServices(flagWith)
	} else {
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
	}

	if len(groups) > 0 {
		if err := huh.NewForm(groups...).Run(); err != nil {
			return err
		}
	}

	cfg.Services = selectedServices

	if !flagNoTools {
		allTools := tool.AllNames()
		cfg.Tools = make([]string, 0)
		for _, t := range allTools {
			switch t {
			case "pint":
				if !flagNoPint {
					cfg.Tools = append(cfg.Tools, t)
				}
			case "larastan":
				if !flagNoLarastan {
					cfg.Tools = append(cfg.Tools, t)
				}
			case "rector":
				if !flagNoRector {
					cfg.Tools = append(cfg.Tools, t)
				}
			case "lefthook":
				if !flagNoLefthook {
					cfg.Tools = append(cfg.Tools, t)
				}
			default:
				cfg.Tools = append(cfg.Tools, t)
			}
		}
	}

	if err := writeConfigAndGenerate(cfg, dir, existingCompose); err != nil {
		return err
	}

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

// --- Shared helpers (moved from init.go, used by both new.go and setup.go) ---

// normalizeLaravelVersion accepts "12", "12.x", or "12.*" and always returns "12.*".
func normalizeLaravelVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimSuffix(v, ".*")
	v = strings.TrimSuffix(v, ".x")
	return v + ".*"
}

// parseServices splits a comma-separated service list and trims whitespace.
func parseServices(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// applyWorkersFromInit builds a cfg.Workers block from init answers.
// queueCount of 0 means no queue pool; a single default pool is added otherwise.
func applyWorkersFromInit(cfg *config.Config, schedule bool, queueCount int) {
	cfg.Workers = config.Workers{}
	if schedule {
		cfg.Workers.Schedule = true
	}
	if queueCount > 0 {
		cfg.Workers.Queue = []config.QueuePool{{
			Name:   "default",
			Queues: []string{"default"},
			Count:  queueCount,
		}}
	}
}

func writeConfigAndGenerate(cfg *config.Config, dir, existingCompose string) error {
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

	stopLaravel := output.Spin("Installing Laravel")
	if err := installLaravel(dir, cfg, true); err != nil {
		stopLaravel(err)
		return err
	}
	stopLaravel(nil)

	// Patch vite.config to import .frank/vite-server.js (known default shape after create-project).
	if err := patchViteConfig(dir); err != nil {
		output.Warning(fmt.Sprintf("could not patch vite.config: %v", err))
	}

	if len(cfg.Tools) > 0 {
		phpTools := tool.PHPTools(cfg.Tools)
		packages := tool.ComposerDevPackages(dir, phpTools)
		if len(packages) > 0 {
			stopReq := output.Spin("Installing dev dependencies")
			if err := composerRequireDev(dir, packages); err != nil {
				stopReq(err)
				output.Warning(fmt.Sprintf("composer require --dev failed: %v", err))
			} else {
				stopReq(nil)
			}
		}

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

func marshalConfig(cfg *config.Config) (string, error) {
	cfg.Version = 1

	// Build a clean ordered map so the YAML field order is predictable.
	type configOutput struct {
		Version  int                             `yaml:"version"`
		PHP      config.PHP                      `yaml:"php"`
		Laravel  config.Laravel                  `yaml:"laravel"`
		Services []string                        `yaml:"services"`
		Config   map[string]config.ServiceConfig `yaml:"config,omitempty"`
		Workers  *config.Workers                 `yaml:"workers,omitempty"`
		Node     *config.Node                    `yaml:"node,omitempty"`
		Tools    []string                        `yaml:"tools,omitempty"`
		Aliases  map[string]config.Alias         `yaml:"aliases,omitempty"`
	}

	out := configOutput{
		Version:  cfg.Version,
		PHP:      cfg.PHP,
		Laravel:  cfg.Laravel,
		Services: cfg.Services,
		Config:   cfg.Config,
	}
	if cfg.Workers.Schedule || len(cfg.Workers.Queue) > 0 {
		w := cfg.Workers
		out.Workers = &w
	}
	if cfg.Node.PackageManager != "" && cfg.Node.PackageManager != "npm" {
		n := cfg.Node
		out.Node = &n
	}
	if len(cfg.Tools) > 0 {
		out.Tools = cfg.Tools
	}
	if len(cfg.Aliases) > 0 {
		out.Aliases = cfg.Aliases
	}

	b, err := yaml.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("marshal frank.yaml: %w", err)
	}
	header := "# Generated by Frank — edit this file to customise your environment\n\n"
	body := header + strings.TrimSpace(string(b)) + "\n"

	comment := buildConfigComment(cfg)
	if comment != "" {
		body += "\n" + comment
	}
	return body, nil
}

// serviceDefaults maps service name to its default port and version for the
// commented-out reference block in frank.yaml.
var serviceDefaults = map[string]config.ServiceConfig{
	"pgsql":       {Port: 5432, Version: "17"},
	"mysql":       {Port: 3306, Version: "latest"},
	"mariadb":     {Port: 3306, Version: "latest"},
	"redis":       {Port: 6379, Version: "alpine"},
	"memcached":   {Port: 11211, Version: "alpine"},
	"meilisearch": {Port: 7700, Version: "latest"},
	"mailpit":     {Port: 8025, Version: "latest"},
}

// buildConfigComment generates a commented-out reference block showing
// available configuration options that are not already set in the active config.
func buildConfigComment(cfg *config.Config) string {
	var sb strings.Builder
	sb.WriteString("# All available configuration options with defaults shown.\n")
	sb.WriteString("# Uncomment and edit as needed.\n")
	sb.WriteString("#\n")
	sb.WriteString("# php.runtime options: frankenphp, fpm\n")
	sb.WriteString("# php.version options: 8.2, 8.3, 8.4, 8.5\n")
	sb.WriteString("# laravel.version options: 12.*, 13.*, latest\n")

	hasExtras := false

	// node — only if not already configured with a non-default value
	if cfg.Node.PackageManager == "" || cfg.Node.PackageManager == "npm" {
		sb.WriteString("#\n")
		sb.WriteString("# node:\n")
		sb.WriteString("#   packageManager: npm       # npm, pnpm, bun\n")
		hasExtras = true
	}

	// config — show defaults for currently selected services (skip sqlite, no config)
	configServices := make([]string, 0)
	for _, svc := range cfg.Services {
		if svc == "sqlite" {
			continue
		}
		// Only show services not already explicitly configured
		if _, already := cfg.Config[svc]; already {
			continue
		}
		if _, ok := serviceDefaults[svc]; ok {
			configServices = append(configServices, svc)
		}
	}
	if len(configServices) > 0 {
		sb.WriteString("#\n")
		sb.WriteString("# config:\n")
		for _, svc := range configServices {
			d := serviceDefaults[svc]
			sb.WriteString(fmt.Sprintf("#   %s:\n", svc))
			sb.WriteString(fmt.Sprintf("#     port: %d\n", d.Port))
			sb.WriteString(fmt.Sprintf("#     version: \"%s\"\n", d.Version))
		}
		hasExtras = true
	}

	// workers — only if not already configured
	if !cfg.Workers.Schedule && len(cfg.Workers.Queue) == 0 {
		sb.WriteString("#\n")
		sb.WriteString("# workers:\n")
		sb.WriteString("#   schedule: true\n")
		sb.WriteString("#   queue:\n")
		sb.WriteString("#     - queues: [default]\n")
		sb.WriteString("#       count: 1\n")
		hasExtras = true
	}

	// tools — only if not already configured
	if len(cfg.Tools) == 0 {
		sb.WriteString("#\n")
		sb.WriteString("# tools:\n")
		sb.WriteString("#   - pint\n")
		sb.WriteString("#   - phpstan\n")
		hasExtras = true
	}

	// aliases — only if not already configured
	if len(cfg.Aliases) == 0 {
		sb.WriteString("#\n")
		sb.WriteString("# aliases:\n")
		sb.WriteString("#   myalias: \"php artisan my:command\"\n")
		hasExtras = true
	}

	_ = hasExtras
	return sb.String()
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
