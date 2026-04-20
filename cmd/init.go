package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/output"
	"github.com/phlisg/frank/internal/tool"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

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

func init() {
	initCmd.Flags().BoolVar(&sailMode, "sail", false, "generate a Sail-compatible project (no Frank traces)")
	initCmd.Flags().StringVar(&flagPHP, "php", "", "PHP version, skips prompt (e.g. 8.5)")
	initCmd.Flags().StringVar(&flagLaravel, "laravel", "", "Laravel version, skips prompt (e.g. 12 or 12.*)")
	initCmd.Flags().StringVar(&flagWith, "with", "", `comma-separated services, skips prompt (e.g. "pgsql,redis,mailpit")`)
	initCmd.Flags().StringVar(&flagRuntime, "runtime", "", "runtime, skips prompt: frankenphp or fpm (ignored with --sail)")
	initCmd.Flags().StringVar(&flagPM, "pm", "", "package manager: npm, pnpm, bun")
	initCmd.Flags().BoolVar(&flagSchedule, "schedule", false, "enable schedule:work worker, skips prompt")
	initCmd.Flags().IntVar(&flagQueueCount, "queue-count", 0, "number of queue:work workers (0-4), skips prompt")
	initCmd.Flags().BoolVar(&flagNoPint, "no-pint", false, "exclude pint from dev tools")
	initCmd.Flags().BoolVar(&flagNoLarastan, "no-larastan", false, "exclude larastan from dev tools")
	initCmd.Flags().BoolVar(&flagNoRector, "no-rector", false, "exclude rector from dev tools")
	initCmd.Flags().BoolVar(&flagNoLefthook, "no-lefthook", false, "exclude lefthook from dev tools")
	initCmd.Flags().BoolVar(&flagNoTools, "no-tools", false, "skip dev tools entirely")
	rootCmd.AddCommand(initCmd)
}

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

var initCmd = &cobra.Command{
	Use:          "init [dirname]",
	Short:        "Interactive wizard to create frank.yaml",
	SilenceUsage: true,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return nil, cobra.ShellCompDirectiveFilterDirs
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	// positionalArg tracks whether the user specified a directory via positional arg
	// (as opposed to --dir). Used later to print a helpful completion message.
	var positionalArg string

	// --dir always wins. Only consider the positional arg when --dir is not set.
	if Dir == "" && len(args) > 0 {
		positionalArg = args[0]
		target := args[0]
		if !filepath.IsAbs(target) {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}
			target = filepath.Join(cwd, target)
		}

		// If the directory exists, it must be empty.
		if info, err := os.Stat(target); err == nil && info.IsDir() {
			entries, err := os.ReadDir(target)
			if err != nil {
				return fmt.Errorf("read directory: %w", err)
			}
			if len(entries) > 0 {
				return fmt.Errorf("directory %q already exists and is not empty", args[0])
			}
		} else if os.IsNotExist(err) {
			if err := os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("create directory: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("stat directory: %w", err)
		}

		// Temporarily set Dir so resolveDir() returns the target path.
		Dir = target
		defer func() { Dir = "" }()
	}

	dir := resolveDir()

	// If --dir was explicitly set and the directory doesn't exist, offer to create it.
	if Dir != "" && positionalArg == "" {
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

	var initErr error
	if sailMode {
		initErr = runSailInit(cfg, dir, existingCompose)
	} else {
		initErr = runFrankInit(cmd, cfg, dir, existingCompose)
	}

	if initErr == nil {
		var steps []string
		if positionalArg != "" {
			steps = append(steps, fmt.Sprintf("cd %s", positionalArg))
		} else if Dir != "" {
			steps = append(steps, fmt.Sprintf("cd %s", Dir))
		}
		steps = append(steps, "frank up -d")
		output.NextSteps(steps)
	}

	return initErr
}

func runFrankInit(cmd *cobra.Command, cfg *config.Config, dir, existingCompose string) error {
	selectedServices := []string{"pgsql", "mailpit"}
	scheduleWorker := flagSchedule
	queueCount := flagQueueCount

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

	// Tool selection
	if !flagNoTools {
		allTools := tool.AllNames()
		// In non-interactive (flag) mode, start with all tools then remove --no-* ones
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
			// Interactive: multi-select with all pre-selected
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

func runSailInit(cfg *config.Config, dir, existingCompose string) error {
	// Sail always uses FPM — no runtime prompt needed.
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

	// Tool selection (non-interactive: all tools unless --no-* flags)
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

	// Step 1: Write frank.yaml, generate .frank/, and install Laravel.
	if err := writeConfigAndGenerate(cfg, dir, existingCompose); err != nil {
		return err
	}

	// Step 2: Install Sail via a second disposable composer container.
	// Running sail:install inside a live container causes inception problems
	// (exit 137/OOM). sail:install only writes files so a disposable container works fine.
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

	output.Detail("generating Docker files")
	if err := generate(cfg, dir); err != nil {
		return err
	}

	if len(cfg.Tools) > 0 {
		output.Detail("installing dev tools")
		res, err := tool.Install(cfg.Tools, dir)
		if err != nil {
			return err
		}
		output.Group("Installed dev tools", fmt.Sprintf("%d created, %d skipped", len(res.Created), len(res.Skipped)))
	}

	if err := runInstall(nil, nil); err != nil {
		return err
	}
	output.Group("Installed Laravel", "")

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
