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

	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/docker"
	"github.com/phlisg/frank/internal/watch"
	"github.com/spf13/cobra"
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
	client := docker.New(dir)

	composeArgs := splitPassthrough(cmd, args)

	// Pre-flight: ensure .frank/ has been generated
	if _, err := os.Stat(filepath.Join(dir, ".frank", "compose.yaml")); os.IsNotExist(err) {
		return fmt.Errorf("no Docker config found — run frank generate first")
	}

	// Pre-flight: detect runtime/PHP version change since last generate
	if stateData, err := os.ReadFile(filepath.Join(dir, ".frank", ".state")); err == nil {
		var state struct {
			PHPVersion string `json:"phpVersion"`
			Runtime    string `json:"runtime"`
		}
		if err := json.Unmarshal(stateData, &state); err == nil {
			cfg, err := config.Load(dir)
			if err == nil {
				if state.PHPVersion != cfg.PHP.Version || state.Runtime != cfg.PHP.Runtime {
					return fmt.Errorf("PHP version or runtime changed since last build — run frank generate && frank up --build")
				}
			}
		}
	}

	detached := upDetach

	// Resolve watcher intent once so fg + -d paths share the decision.
	cfg, _ := config.Load(dir)
	wantWatcher := shouldRunWatcher(cfg, client, dir)

	// Foreground mode: spawn watcher goroutine BEFORE compose so a .php
	// edit during container boot still lands a reload trigger once the
	// arm-suppression window clears. SIGINT/SIGTERM cancels both.
	var stopWatcher func() error
	if !detached && wantWatcher {
		var err error
		stopWatcher, err = startForegroundWatcher(dir, cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: watcher not started: %v\n", err)
		}
	}

	upErr := client.Up(composeArgs...)

	if stopWatcher != nil {
		if err := stopWatcher(); err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintf(os.Stderr, "warning: watcher stopped with error: %v\n", err)
		}
	}

	if upErr != nil {
		return upErr
	}

	if upQuick {
		return nil
	}

	// Wait for laravel.test to be ready before running post-start tasks.
	// Only meaningful in detached mode; in foreground mode Up() never returns here.
	fmt.Println("Waiting for laravel.test to be ready...")
	if err := client.WaitForContainer("laravel.test", 30*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v — skipping post-start tasks\n", err)
		return nil
	}

	// Post-start tasks — failures are logged but don't abort.
	if _, err := os.Stat(filepath.Join(dir, "composer.json")); err == nil {
		if err := client.Exec("laravel.test", "composer", "install", "--no-interaction"); err != nil {
			fmt.Fprintf(os.Stderr, "warning: composer install failed: %v\n", err)
		}
	}

	if _, err := os.Stat(filepath.Join(dir, "artisan")); err == nil {
		if err := client.Exec("laravel.test", "php", "artisan", "migrate", "--force"); err != nil {
			fmt.Fprintf(os.Stderr, "warning: artisan migrate failed: %v\n", err)
		}
	}

	// npm install is intentionally not run automatically — it is memory-intensive
	// and can OOM the container. Run it manually when needed:
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
		fmt.Println("  npm install   # install frontend dependencies")
		fmt.Println("  npm run dev   # start Vite dev server")
	}

	// -d mode: after laravel.test is healthy and post-start migrations
	// have run, spawn a detached `frank watch` child. The child acquires
	// .frank/watch.pid via its own Start — we don't pre-write it here.
	if detached && wantWatcher {
		if err := spawnDetachedWatcher(dir); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not start watcher: %v\n", err)
		}
	}

	return nil
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

	fmt.Fprintf(os.Stderr, "frank watch: foreground (pid %d) armed with containers\n", os.Getpid())

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
	fmt.Fprintf(os.Stderr, "frank watch: detached child started (pid %d) — logs at %s\n",
		pid, watch.LogfilePath(projectRoot))
	return nil
}
