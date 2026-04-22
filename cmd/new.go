package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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

		printNewNextSteps(projectName, !flagNoUp)
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
	printNewNextSteps(projectName, !flagNoUp)
	return nil
}

// runNewUp runs doUp then npm install inside the container.
func runNewUp(dir string, cfg *config.Config) error {
	if err := doUp(dir, true, false, nil); err != nil {
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

		output.Detail(fmt.Sprintf("running %s install", pm))

		// Wait for container to be ready
		if err := client.WaitForContainer("laravel.test", 30*time.Second); err != nil {
			output.Warning(fmt.Sprintf("container not ready for %s install: %v", pm, err))
			return nil
		}

		if err := client.Exec("laravel.test", pm, "install"); err != nil {
			output.Warning(fmt.Sprintf("%s install failed: %v", pm, err))
		} else {
			output.Group("Installed npm dependencies", "")
		}
	}

	return nil
}

// printNewNextSteps prints the final NextSteps block for frank new.
func printNewNextSteps(projectName string, containersStarted bool) {
	steps := []string{fmt.Sprintf("cd %s", projectName)}
	if containersStarted {
		steps = append(steps, "http://localhost")
	} else {
		steps = append(steps, "frank up -d")
	}
	steps = append(steps, "")
	steps = append(steps, "Tip: run `frank activate` to load shell aliases (up, down, artisan, etc.)")
	output.NextSteps(steps)
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

	output.Detail("generating Docker files")
	if err := generate(cfg, dir); err != nil {
		return err
	}
	output.Group("Generated Docker files", fmt.Sprintf("%d files", generatedFileCount(cfg)))

	if err := installLaravel(dir, cfg, true); err != nil {
		return err
	}
	output.Group("Installed Laravel", "")

	if len(cfg.Tools) > 0 {
		// Install composer dev dependencies via docker (updates both json + lock).
		phpTools := tool.PHPTools(cfg.Tools)
		packages := tool.ComposerDevPackages(dir, phpTools)
		if len(packages) > 0 {
			if err := composerRequireDev(dir, packages); err != nil {
				output.Warning(fmt.Sprintf("composer require --dev failed: %v", err))
			}
		}

		output.Detail("installing dev tools")
		res, err := tool.Install(cfg.Tools, dir)
		if err != nil {
			return err
		}
		output.Group("Installed dev tools", fmt.Sprintf("%d created, %d skipped", len(res.Created), len(res.Skipped)))
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
