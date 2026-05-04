package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/phlisg/frank/internal/cert"
	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/docker"
	"github.com/phlisg/frank/internal/output"
	"github.com/phlisg/frank/internal/watch"
	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
)

var (
	upDetach bool
	upQuick  bool
)

func init() {
	upCmd.Flags().BoolVarP(&upDetach, "detach", "d", false, "Run containers in the background")
	upCmd.Flags().BoolVar(&upQuick, "quick", false, "Skip post-start tasks (composer install + artisan migrate)")
	upCmd.SetFlagErrorFunc(upFlagError)
	rootCmd.AddCommand(upCmd)
}

var upCmd = &cobra.Command{
	Use:   "up [-d] [--quick] [-- <compose args>]",
	Short: "Start containers (-d / --quick frank-owned; compose flags after --)",
	Long: `Start containers. Frank owns -d/--detach and --quick because it
needs those decisions for watcher spawn and post-start task skipping.
Every other docker compose flag must come after a literal "--".

Examples:
  frank up                              # foreground
  frank up -d                           # detached
  frank up --quick                      # skip post-start tasks
  frank up -- --build                   # force rebuild
  frank up -d -- --force-recreate       # detached + compose flag`,
	SilenceUsage:      true,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE:              runUp,
}

// upFlagError turns "unknown flag: --foo" into an actionable hint that
// points the user at the `--` separator for docker compose passthrough.
func upFlagError(cmd *cobra.Command, err error) error {
	return fmt.Errorf("%w\n\nHint: frank up only owns -d/--detach and --quick — pass docker compose flags after `--`\n  frank up -- --build", err)
}

func runUp(cmd *cobra.Command, args []string) error {
	dir := resolveDir()

	composeArgs := splitPassthrough(cmd, args)

	return doUp(dir, upDetach, upQuick, composeArgs, true)
}

func doUp(dir string, detach, quick bool, passthrough []string, showNextSteps bool) error {
	client := docker.New(dir)

	composeArgs := passthrough
	if detach {
		composeArgs = append([]string{"-d"}, composeArgs...)
	}

	// Pre-flight: ensure .frank/ has been generated
	if _, err := os.Stat(filepath.Join(dir, ".frank", "compose.yaml")); os.IsNotExist(err) {
		return fmt.Errorf("no Docker config found — run frank generate first")
	}

	// Pre-flight: detect stale .frank/ (version mismatch or config change) and auto-regenerate.
	if regenerated, err := autoRegenerate(dir, rootCmd.Version); err != nil {
		return err
	} else if regenerated {
		composeArgs = append(composeArgs, "--build")
		quick = false
	}

	// Resolve watcher intent once so fg + -d paths share the decision.
	cfg, _ := config.Load(dir)

	// HTTPS nudge: warn if HTTPS enabled but certs missing.
	if cfg != nil && cfg.Server.IsHTTPS() {
		frankDir := filepath.Join(dir, ".frank")
		if !cert.CertsExist(frankDir) {
			if cert.MkcertAvailable() {
				output.Warning("mkcert found but no certs generated. Run `frank generate && frank up --build` to enable HTTPS.")
			} else {
				output.Warning("HTTPS enabled but mkcert not found. Install mkcert and run `frank generate`, or set `server.https: false` in frank.yaml.\n  https://github.com/FiloSottile/mkcert#installation")
			}
		}
	}

	wantWatcher := shouldRunWatcher(cfg, client, dir)

	// Foreground mode: spawn watcher goroutine BEFORE compose so a .php
	// edit during container boot still lands a reload trigger once the
	// arm-suppression window clears. SIGINT/SIGTERM cancels both.
	var stopWatcher func() error
	if !detach && wantWatcher {
		var err error
		stopWatcher, err = startForegroundWatcher(dir, cfg)
		if err != nil {
			output.Warning(fmt.Sprintf("watcher not started: %v", err))
		}
	}

	stopSpin := output.Spin("Starting containers")
	var upErr error
	if output.GetLevel() == output.Verbose {
		upErr = client.Up(composeArgs...)
	} else {
		_, upErr = client.RunQuiet(append([]string{"up"}, composeArgs...)...)
	}

	if stopWatcher != nil {
		if err := stopWatcher(); err != nil && !errors.Is(err, context.Canceled) {
			output.Warning(fmt.Sprintf("watcher stopped with error: %v", err))
		}
	}

	stopSpin(upErr)
	if upErr != nil {
		return upErr
	}

	if quick {
		return nil
	}

	stopPost := output.Spin("Running post-start tasks")
	output.Detail("waiting for laravel.test to be ready")
	if err := client.WaitForContainer("laravel.test", 30*time.Second); err != nil {
		output.Warning(fmt.Sprintf("%v — skipping post-start tasks", err))
		stopPost(nil)
		return nil
	}

	if _, err := os.Stat(filepath.Join(dir, "composer.json")); err == nil {
		if output.GetLevel() == output.Verbose {
			if err := client.Exec("laravel.test", "composer", "install", "--no-interaction"); err != nil {
				output.Warning(fmt.Sprintf("composer install failed: %v", err))
			}
		} else {
			if _, err := client.ExecQuiet("laravel.test", "composer", "install", "--no-interaction"); err != nil {
				output.Warning(fmt.Sprintf("composer install failed: %v", err))
			}
		}
	}

	if _, err := os.Stat(filepath.Join(dir, "artisan")); err == nil {
		if output.GetLevel() == output.Verbose {
			if err := client.Exec("laravel.test", "php", "artisan", "migrate", "--force"); err != nil {
				output.Warning(fmt.Sprintf("artisan migrate failed: %v", err))
			}
		} else {
			if _, err := client.ExecQuiet("laravel.test", "php", "artisan", "migrate", "--force"); err != nil {
				output.Warning(fmt.Sprintf("artisan migrate failed: %v", err))
			}
		}
	}

	stopPost(nil)

	if detach && showNextSteps {
		var steps []string
		pm := "npm"
		if cfg != nil && cfg.Node.PackageManager != "" {
			pm = cfg.Node.PackageManager
		}
		if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
			steps = append(steps, fmt.Sprintf("%s install && %s run dev", pm, pm))
		}
		output.NextSteps(steps)
	}

	if detach && wantWatcher {
		if err := spawnDetachedWatcher(dir); err != nil {
			output.Warning(fmt.Sprintf("could not start watcher: %v", err))
		}
	}

	return nil
}

// autoRegenerate checks .frank/.state for staleness (frank version mismatch
// or PHP version/runtime change) and regenerates if needed. Returns true if
// regeneration occurred.
func autoRegenerate(dir, currentVersion string) (bool, error) {
	stateFile := filepath.Join(dir, ".frank", ".state")
	stateData, err := os.ReadFile(stateFile)

	var state struct {
		PHPVersion   string `json:"phpVersion"`
		Runtime      string `json:"runtime"`
		FrankVersion string `json:"frankVersion"`
	}

	stale := false
	reason := ""

	if err != nil {
		// No .state file — existing project upgrading to this frank version.
		stale = true
		reason = ".frank/.state missing"
	} else if err := json.Unmarshal(stateData, &state); err != nil {
		stale = true
		reason = ".frank/.state corrupt"
	} else {
		// Check frank version staleness.
		if currentVersion == "dev" {
			stale = true
			reason = "dev build"
		} else if state.FrankVersion == "" {
			stale = true
			reason = "frank version not stamped"
		} else if state.FrankVersion != "dev" {
			vc := "v" + currentVersion
			vs := "v" + state.FrankVersion
			if semver.IsValid(vc) && semver.IsValid(vs) && semver.Compare(vc, vs) > 0 {
				stale = true
				reason = fmt.Sprintf("frank updated %s → %s", state.FrankVersion, currentVersion)
			}
		}

		// Check PHP version/runtime change (even if frank version matches).
		if !stale {
			cfg, err := config.Load(dir)
			if err == nil {
				if state.PHPVersion != cfg.PHP.Version || state.Runtime != cfg.PHP.Runtime {
					stale = true
					reason = "PHP version or runtime changed"
				}
			}
		}
	}

	if !stale {
		return false, nil
	}

	// Need to regenerate — load config.
	cfg, err := config.Load(dir)
	if err != nil {
		// Config can't load — skip auto-regen, let normal flow handle it.
		output.Detail(fmt.Sprintf("skipping auto-regenerate (%s): %v", reason, err))
		return false, nil
	}

	stopGen := output.Spin(fmt.Sprintf("Regenerating .frank/ (%s)", reason))
	if err := generate(cfg, dir, currentVersion); err != nil {
		stopGen(err)
		return false, fmt.Errorf("auto-regenerate failed: %w", err)
	}
	stopGen(nil)

	return true, nil
}

// shouldRunWatcher decides whether `frank up` should spawn a watcher.
// Returns true when schedule is enabled, any queue pool is declared, or
// any ad-hoc worker is already running under this project. Degrades to
// false (no watcher) on load/docker errors — worker reload is
// nice-to-have, not a reason to block `frank up`.
func shouldRunWatcher(cfg *config.Config, client *docker.Client, projectRoot string) bool {
	if cfg != nil {
		if cfg.Workers.Schedule {
			return true
		}
		if totalQueueCount(cfg) > 0 {
			return true
		}
	}
	if client != nil {
		project := config.ProjectName(projectRoot)
		if names, err := client.AdhocWorkerNames(project); err == nil && len(names) > 0 {
			return true
		}
	}
	return false
}

// startForegroundWatcher spins up an in-process watcher goroutine that
// shares the parent process's lifecycle. Returns a stop function the
// caller invokes once the compose run returns (Ctrl-C or normal exit).
// Nil cfg is tolerated — callers warn separately on a missing frank.yaml.
func startForegroundWatcher(projectRoot string, cfg *config.Config) (func() error, error) {
	if cfg == nil {
		return nil, fmt.Errorf("frank.yaml missing — skipping watcher")
	}

	w, err := watch.New(watch.Config{
		ProjectRoot:       projectRoot,
		ScheduleEnabled:   cfg.Workers.Schedule,
		QueueCount:        totalQueueCount(cfg),
		DockerComposeFile: ".frank/compose.yaml",
		ArmSuppression:    watch.DefaultArmSuppression,
	})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	done := make(chan error, 1)
	go func() { done <- w.Start(ctx) }()

	output.Detail(fmt.Sprintf("frank watch: foreground (pid %d) armed", os.Getpid()))

	return func() error {
		signal.Stop(sigCh)
		cancel()
		err := <-done
		return err
	}, nil
}

// spawnDetachedWatcher forks the current binary as `frank watch` detached
// from the controlling terminal. Stdout/stderr land in .frank/watch.log.
// The spawned child writes its own pidfile via Start.
func spawnDetachedWatcher(projectRoot string) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	argv := []string{self, "--dir", projectRoot, "watch"}
	pid, err := watch.Daemonize(argv, watch.LogfilePath(projectRoot))
	if err != nil {
		return err
	}
	output.Detail(fmt.Sprintf("frank watch: detached child (pid %d)", pid))
	return nil
}
