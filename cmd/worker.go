package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/docker"
	"github.com/spf13/cobra"
)

// Shared queue flags for `frank worker queue`.
var (
	workerQueueCount   int
	workerQueueQueue   string
	workerQueueTries   int
	workerQueueTimeout int
	workerQueueMemory  int
	workerQueueSleep   int
	workerQueueBackoff int

	workerStopAll bool

	workerLogsFollow bool
)

func init() {
	// queue
	workerQueueCmd.Flags().IntVar(&workerQueueCount, "count", 1, "Number of ad-hoc queue workers to spawn")
	workerQueueCmd.Flags().StringVar(&workerQueueQueue, "queue", "default", "Queue(s) to consume (comma-separated)")
	workerQueueCmd.Flags().IntVar(&workerQueueTries, "tries", 0, "Max attempts per job (0 = artisan default)")
	workerQueueCmd.Flags().IntVar(&workerQueueTimeout, "timeout", 0, "Per-job timeout in seconds (0 = artisan default)")
	workerQueueCmd.Flags().IntVar(&workerQueueMemory, "memory", 0, "Memory limit in MB (0 = artisan default)")
	workerQueueCmd.Flags().IntVar(&workerQueueSleep, "sleep", 0, "Sleep seconds when idle (0 = artisan default)")
	workerQueueCmd.Flags().IntVar(&workerQueueBackoff, "backoff", 0, "Backoff seconds on failure (0 = artisan default)")

	// stop
	workerStopCmd.Flags().BoolVar(&workerStopAll, "all", false, "Also stop declared workers (not just ad-hoc)")

	// logs
	workerLogsCmd.Flags().BoolVarP(&workerLogsFollow, "follow", "f", false, "Follow log output")

	workerCmd.AddCommand(workerQueueCmd)
	workerCmd.AddCommand(workerScheduleCmd)
	workerCmd.AddCommand(workerListCmd)
	workerCmd.AddCommand(workerStopCmd)
	workerCmd.AddCommand(workerLogsCmd)

	rootCmd.AddCommand(workerCmd)
}

var workerCmd = &cobra.Command{
	Use:               "worker",
	Short:             "Manage Laravel schedule + queue workers",
	SilenceUsage:      true,
	ValidArgsFunction: cobra.NoFileCompletions,
}

var workerQueueCmd = &cobra.Command{
	Use:               "queue [-- <artisan flags>]",
	Short:             "Spawn ad-hoc queue workers",
	SilenceUsage:      true,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE:              runWorkerQueue,
}

var workerScheduleCmd = &cobra.Command{
	Use:               "schedule",
	Short:             "Spawn an ad-hoc schedule:work container",
	SilenceUsage:      true,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE:              runWorkerSchedule,
}

var workerListCmd = &cobra.Command{
	Use:               "list",
	Short:             "List worker containers (declared + ad-hoc)",
	SilenceUsage:      true,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE:              runWorkerList,
}

var workerStopCmd = &cobra.Command{
	Use:               "stop",
	Short:             "Stop ad-hoc workers (use --all to also stop declared workers)",
	SilenceUsage:      true,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE:              runWorkerStop,
}

var workerLogsCmd = &cobra.Command{
	Use:               "logs [name]",
	Short:             "Tail logs for workers",
	Args:              cobra.MaximumNArgs(1),
	SilenceUsage:      true,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE:              runWorkerLogs,
}

// buildQueueArtisanArgs translates the per-pool flag set into the artisan
// command arg slice: ["php", "artisan", "queue:work", "--queue=...", ...].
// Only emits flags whose value is > 0 (except --queue which is always set).
// passthrough is appended verbatim at the end.
func buildQueueArtisanArgs(queue string, tries, timeout, memory, sleep, backoff int, passthrough []string) []string {
	args := []string{"php", "artisan", "queue:work", "--queue=" + queue}
	if tries > 0 {
		args = append(args, fmt.Sprintf("--tries=%d", tries))
	}
	if timeout > 0 {
		args = append(args, fmt.Sprintf("--timeout=%d", timeout))
	}
	if memory > 0 {
		args = append(args, fmt.Sprintf("--memory=%d", memory))
	}
	if sleep > 0 {
		args = append(args, fmt.Sprintf("--sleep=%d", sleep))
	}
	if backoff > 0 {
		args = append(args, fmt.Sprintf("--backoff=%d", backoff))
	}
	args = append(args, passthrough...)
	return args
}

// adhocQueueName returns the ad-hoc queue worker container name for index i
// (1-based) at the given epoch.
func adhocQueueName(epoch int64, i int) string {
	return fmt.Sprintf("laravel.queue.adhoc.%d.%d", epoch, i)
}

// adhocScheduleName returns the ad-hoc schedule container name at the given
// epoch.
func adhocScheduleName(epoch int64) string {
	return fmt.Sprintf("laravel.schedule.adhoc.%d", epoch)
}

// splitArgs separates user positional args from passthrough args after "--".
// Cobra normally strips the literal "--" token from args while reporting the
// split position via ArgsLenAtDash. We still scan defensively in case the
// token survives (it does in some cobra versions / with DisableFlagParsing).
func splitArgs(args []string) []string {
	for i, a := range args {
		if a == "--" {
			return args[i+1:]
		}
	}
	return args
}

// passthroughFromCobra returns args after a literal `--` separator, using
// cobra's ArgsLenAtDash when available and falling back to splitArgs. Called
// from the RunE handler so it can access the cobra.Command.
func passthroughFromCobra(cmd *cobra.Command, args []string) []string {
	// When flag parsing is enabled, cobra reports the index in args where
	// "--" appeared. Args with index >= dash are positional-after-dash.
	dash := cmd.ArgsLenAtDash()
	if dash >= 0 && dash <= len(args) {
		return args[dash:]
	}
	return splitArgs(args)
}

func runWorkerQueue(cmd *cobra.Command, args []string) error {
	dir := resolveDir()
	projectName := config.ProjectName(dir)
	client := docker.New(dir)

	if workerQueueCount < 1 {
		return fmt.Errorf("--count must be >= 1")
	}

	passthrough := passthroughFromCobra(cmd, args)
	epoch := time.Now().Unix()

	labels := map[string]string{
		"frank.project":     projectName,
		"frank.worker":      "adhoc",
		"frank.worker.type": "queue",
	}

	cmdArgs := buildQueueArtisanArgs(
		workerQueueQueue,
		workerQueueTries,
		workerQueueTimeout,
		workerQueueMemory,
		workerQueueSleep,
		workerQueueBackoff,
		passthrough,
	)

	for i := 1; i <= workerQueueCount; i++ {
		name := adhocQueueName(epoch, i)
		if err := client.RunAdhoc(name, labels, cmdArgs); err != nil {
			return fmt.Errorf("spawn %s: %w", name, err)
		}
		fmt.Println(name)
	}

	// TODO(td-1a786c): auto-spawn watcher here if none running. Hook into
	// internal/watch once that lands; for now, leave a user-visible notice.
	fmt.Println("frank worker queue: code reload not armed — run 'frank watch' or 'frank up' to start the watcher")
	return nil
}

func runWorkerSchedule(cmd *cobra.Command, args []string) error {
	dir := resolveDir()
	projectName := config.ProjectName(dir)
	client := docker.New(dir)

	// Refuse to spawn a second ad-hoc schedule for this project.
	out, err := client.ListContainers(projectName, "adhoc", "{{.Names}}\t{{.Label \"frank.worker.type\"}}")
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if line == "" {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) >= 2 && parts[1] == "schedule" {
				return fmt.Errorf("ad-hoc schedule already running: %s", parts[0])
			}
		}
	}

	epoch := time.Now().Unix()
	name := adhocScheduleName(epoch)
	labels := map[string]string{
		"frank.project":     projectName,
		"frank.worker":      "adhoc",
		"frank.worker.type": "schedule",
	}
	cmdArgs := []string{"php", "artisan", "schedule:work"}

	if err := client.RunAdhoc(name, labels, cmdArgs); err != nil {
		return fmt.Errorf("spawn %s: %w", name, err)
	}
	fmt.Println(name)
	return nil
}

func runWorkerList(cmd *cobra.Command, args []string) error {
	dir := resolveDir()
	projectName := config.ProjectName(dir)
	client := docker.New(dir)

	format := "table {{.Names}}\t{{.Label \"frank.worker\"}}\t{{.Label \"frank.worker.pool\"}}\t{{.Command}}\t{{.RunningFor}}\t{{.Status}}"
	out, err := client.ListContainers(projectName, "", format)
	if err != nil {
		fmt.Printf("No worker containers found for project %s.\n", projectName)
		return nil
	}

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	// docker ps with --format table always emits a header. Header-only = no matches.
	if len(lines) <= 1 {
		fmt.Printf("No worker containers found for project %s.\n", projectName)
		return nil
	}

	// Replace default header with our column names (TYPE=frank.worker, POOL=frank.worker.pool).
	lines[0] = "NAME\tTYPE\tPOOL\tCOMMAND\tUPTIME\tSTATUS"
	fmt.Println(strings.Join(lines, "\n"))
	return nil
}

func runWorkerStop(cmd *cobra.Command, args []string) error {
	dir := resolveDir()
	projectName := config.ProjectName(dir)
	client := docker.New(dir)

	// Always collect ad-hoc names (force-removed via `docker rm -f`).
	adhocOut, _ := client.ListContainers(projectName, "adhoc", "{{.Names}}")
	var adhocNames []string
	for _, line := range strings.Split(strings.TrimSpace(adhocOut), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			adhocNames = append(adhocNames, line)
		}
	}

	if len(adhocNames) > 0 {
		if err := client.StopContainers(adhocNames); err != nil {
			return fmt.Errorf("stop ad-hoc workers: %w", err)
		}
		for _, n := range adhocNames {
			fmt.Println(n)
		}
	}

	if !workerStopAll {
		if len(adhocNames) == 0 {
			fmt.Println("No ad-hoc workers running.")
		}
		return nil
	}

	// Declared workers: stopped via compose (managed services). Collect names
	// and hand them to `docker compose stop`.
	declaredOut, err := client.ListContainers(projectName, "declared", "{{.Names}}")
	if err != nil {
		return nil
	}
	var declaredNames []string
	for _, line := range strings.Split(strings.TrimSpace(declaredOut), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && line != "laravel.test" {
			declaredNames = append(declaredNames, line)
		}
	}
	if len(declaredNames) == 0 {
		if len(adhocNames) == 0 {
			fmt.Println("No workers running.")
		}
		return nil
	}
	stopArgs := append([]string{"stop"}, declaredNames...)
	if err := client.Run(stopArgs...); err != nil {
		return fmt.Errorf("stop declared workers: %w", err)
	}
	for _, n := range declaredNames {
		fmt.Println(n)
	}
	return nil
}

func runWorkerLogs(cmd *cobra.Command, args []string) error {
	dir := resolveDir()
	projectName := config.ProjectName(dir)
	client := docker.New(dir)

	if len(args) == 1 {
		name := args[0]
		// Declared workers live under compose; ad-hoc ones may not, so detect.
		if client.ComposePSServiceExists(name) {
			return client.LogsForWorkers([]string{name}, workerLogsFollow)
		}
		return client.LogsRaw(name, workerLogsFollow)
	}

	// No name: collect declared worker service names and tail their compose logs.
	out, err := client.ListContainers(projectName, "declared", "{{.Names}}")
	if err != nil {
		fmt.Printf("No declared workers for project %s.\n", projectName)
		return nil
	}
	var services []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		services = append(services, line)
	}
	if len(services) == 0 {
		fmt.Printf("No declared workers for project %s.\n", projectName)
		return nil
	}
	return client.LogsForWorkers(services, workerLogsFollow)
}
