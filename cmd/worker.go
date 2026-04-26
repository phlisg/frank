package cmd

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/docker"
	"github.com/phlisg/frank/internal/watch"
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
	Use:               "ps",
	Aliases:           []string{"list"},
	Short:             "Show worker containers (declared + ad-hoc)",
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

func runWorkerQueue(cmd *cobra.Command, args []string) error {
	dir := resolveDir()
	projectName := config.ProjectName(dir)
	client := docker.New(dir)

	if workerQueueCount < 1 {
		return fmt.Errorf("--count must be >= 1")
	}

	passthrough := splitPassthrough(cmd, args)
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

	printWatcherHintIfNeeded(dir, client)
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

	printWatcherHintIfNeeded(dir, client)
	return nil
}

// printWatcherHintIfNeeded emits a reload hint only when no watcher is
// running for this project. A live watcher means reload is already armed
// for any ad-hoc container (queue:restart + schedule restart fire for
// every queue worker in the compose project, declared or ad-hoc).
//
// Uses watch.StatusChecker so an orphaned watcher (pid alive, laravel
// gone) is cleaned up as a side effect — the user's next action then
// starts from a known-good state.
func printWatcherHintIfNeeded(projectRoot string, client *docker.Client) {
	checker := watch.NewStatusChecker(projectRoot, func() bool {
		return client.ComposePSServiceExists("laravel.test")
	})
	st, _ := checker.Check()
	switch st.State {
	case watch.StatusRunning:
		// Nothing to say — reload is armed.
	case watch.StatusOrphaned:
		fmt.Println("frank worker: orphaned watcher cleaned up; run `frank watch` to rearm code reload")
	default:
		fmt.Println("frank worker: code reload not armed — run `frank watch` or `frank up` to start the watcher")
	}
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
		// Declared workers live under compose; ad-hoc ones don't, so detect.
		if client.ComposePSServiceExists(name) {
			return client.LogsForWorkers([]string{name}, workerLogsFollow)
		}
		return client.LogsRaw(name, workerLogsFollow)
	}

	declared, adhoc, err := listWorkerNames(client, projectName)
	if err != nil || (len(declared) == 0 && len(adhoc) == 0) {
		fmt.Printf("No workers for project %s.\n", projectName)
		return nil
	}

	// Fast paths: single-backend invocations keep the native log UX.
	if len(adhoc) == 0 {
		return client.LogsForWorkers(declared, workerLogsFollow)
	}
	if len(declared) == 0 && len(adhoc) == 1 && !workerLogsFollow {
		return client.LogsRaw(adhoc[0], workerLogsFollow)
	}

	// Mixed or multi-adhoc: fan out. Compose handles its own multi-service
	// tail; each ad-hoc container gets its own goroutine with a line prefix
	// so interleaved output stays readable.
	return streamMixedWorkerLogs(client, declared, adhoc, workerLogsFollow)
}

// listWorkerNames partitions the project's worker containers into declared
// (managed by compose) and ad-hoc (started via `compose run -d`). Both
// lists preserve docker's output order so the UX is predictable.
func listWorkerNames(client *docker.Client, projectName string) (declared, adhoc []string, err error) {
	out, err := client.ListContainers(projectName, "", "{{.Names}}\t{{.Label \"frank.worker\"}}")
	if err != nil {
		return nil, nil, err
	}
	declared, adhoc = parseWorkerList(out)
	return declared, adhoc, nil
}

// parseWorkerList splits docker ps output (one "<name>\t<frank.worker>"
// line per container) into declared and ad-hoc names. Lines without a
// label kind default to declared so containers predating the label scheme
// stay addressable; ad-hoc classification requires an explicit "adhoc".
func parseWorkerList(out string) (declared, adhoc []string) {
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		name := strings.TrimSpace(parts[0])
		if name == "" {
			continue
		}
		kind := ""
		if len(parts) == 2 {
			kind = strings.TrimSpace(parts[1])
		}
		if kind == "adhoc" {
			adhoc = append(adhoc, name)
		} else {
			declared = append(declared, name)
		}
	}
	return declared, adhoc
}

// streamMixedWorkerLogs fans a mixed set of declared + ad-hoc workers out
// to their respective backends. `docker compose logs` handles the declared
// subset natively (it already prefixes lines with the service name). Each
// ad-hoc container is streamed via LogsRawPrefixed in its own goroutine
// so the prefix form matches. Blocks until every backend exits.
func streamMixedWorkerLogs(client *docker.Client, declared, adhoc []string, follow bool) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(adhoc)+1)

	if len(declared) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := client.LogsForWorkers(declared, follow); err != nil {
				errs <- fmt.Errorf("declared logs: %w", err)
			}
		}()
	}
	for _, name := range adhoc {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			if err := client.LogsRawPrefixed(n, follow); err != nil {
				errs <- fmt.Errorf("%s logs: %w", n, err)
			}
		}(name)
	}

	wg.Wait()
	close(errs)
	var firstErr error
	for e := range errs {
		if firstErr == nil {
			firstErr = e
		}
	}
	return firstErr
}
