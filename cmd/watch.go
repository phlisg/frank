package cmd

import (
	"context"
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
	watchStop   bool
	watchStatus bool
)

func init() {
	watchCmd.Flags().BoolVar(&watchStop, "stop", false, "SIGTERM the detached watcher and unlink the pidfile")
	watchCmd.Flags().BoolVar(&watchStatus, "status", false, "Print the watcher's PID, uptime, and liveness state")
	rootCmd.AddCommand(watchCmd)
}

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Run the host-side file watcher that reloads Laravel workers on code change",
	Long: `Runs the host-side file watcher in the foreground. Edits under app/,
bootstrap/, config/, database/, lang/, resources/views/, routes/, .env,
or composer.lock trigger 'php artisan queue:restart' and (when
workers.schedule is enabled) a compose restart of the schedule service.

Default invocation errors if another watcher is already running — use
--stop to SIGTERM the detached one first, or --status to inspect it.`,
	SilenceUsage:      true,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE:              runWatch,
}

func runWatch(cmd *cobra.Command, args []string) error {
	if watchStop && watchStatus {
		return fmt.Errorf("--stop and --status are mutually exclusive")
	}

	dir := resolveDir()

	switch {
	case watchStop:
		return runWatchStop(dir)
	case watchStatus:
		return runWatchStatus(dir)
	default:
		return runWatchForeground(dir)
	}
}

func runWatchForeground(projectRoot string) error {
	if _, err := os.Stat(filepath.Join(projectRoot, ".frank", "compose.yaml")); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("no Docker config found — run frank generate first")
	}

	cfg, err := config.Load(projectRoot)
	if err != nil {
		return fmt.Errorf("load frank.yaml: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Signal handling: SIGINT/SIGTERM cancels the context so Start
	// returns cleanly, flushing any in-flight dispatch.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	w, err := watch.New(watch.Config{
		ProjectRoot:       projectRoot,
		ScheduleEnabled:   cfg.Workers.Schedule,
		QueueCount:        totalQueueCount(cfg),
		DockerComposeFile: ".frank/compose.yaml",
		ArmSuppression:    watch.DefaultArmSuppression,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "frank watch: foreground (pid %d) — Ctrl-C to stop\n", os.Getpid())
	if err := w.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func runWatchStop(projectRoot string) error {
	path := watch.PidfilePath(projectRoot)
	pid, err := watch.ReadPidfile(path)
	if err != nil {
		// Malformed pidfile: unlink + report.
		_ = os.Remove(path)
		return fmt.Errorf("stale or malformed pidfile removed: %w", err)
	}
	if pid == 0 {
		fmt.Println("frank watch: no watcher running")
		return nil
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			_ = os.Remove(path)
			fmt.Printf("frank watch: stale pidfile (pid %d not running) removed\n", pid)
			return nil
		}
		return fmt.Errorf("SIGTERM %d: %w", pid, err)
	}

	// Give the watcher a moment to release the pidfile on its way out.
	// If it hasn't cleaned up by the deadline, clean up for it.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			fmt.Printf("frank watch: stopped (pid %d)\n", pid)
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = os.Remove(path)
	fmt.Printf("frank watch: sent SIGTERM to pid %d; pidfile force-unlinked after timeout\n", pid)
	return nil
}

func runWatchStatus(projectRoot string) error {
	projectName := config.ProjectName(projectRoot)
	client := docker.New(projectRoot)

	checker := watch.NewStatusChecker(projectRoot, func() bool {
		return client.ComposePSServiceExists("laravel.test")
	})

	st, err := checker.Check()
	if err != nil {
		fmt.Fprintf(os.Stderr, "frank watch: WARN %v\n", err)
	}

	fmt.Printf("project:  %s\n", projectName)
	fmt.Printf("state:    %s\n", st.State)
	if st.PID != 0 {
		fmt.Printf("pid:      %d\n", st.PID)
	}
	if !st.StartedAt.IsZero() {
		fmt.Printf("uptime:   %s (started %s)\n",
			formatUptime(st.Uptime()),
			st.StartedAt.Format(time.RFC3339),
		)
	}
	fmt.Printf("pidfile:  %s\n", watch.PidfilePath(projectRoot))
	fmt.Printf("logfile:  %s\n", watch.LogfilePath(projectRoot))
	fmt.Println()
	fmt.Println(".gitignore edit? restart `frank watch` — new ignore rules only apply after a re-arm.")
	return nil
}

// totalQueueCount sums the declared queue worker count across all pools.
// Used purely to inform the watcher whether there's anything to reload;
// actual container names come from compose.
func totalQueueCount(cfg *config.Config) int {
	total := 0
	for _, pool := range cfg.Workers.Queue {
		if pool.Count > 0 {
			total += pool.Count
		}
	}
	return total
}

// formatUptime renders a duration compactly: "3m12s", "1h04m", "2d03h".
func formatUptime(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
	days := int(d / (24 * time.Hour))
	rem := d - time.Duration(days)*24*time.Hour
	return fmt.Sprintf("%dd%02dh", days, int(rem.Hours()))
}
