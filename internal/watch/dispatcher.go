package watch

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Runner executes a single trigger (queue:restart or schedule restart) against
// a running compose project. Split out so tests can inject a fake without
// shelling out to docker.
type Runner interface {
	Trigger(ctx context.Context, kind TriggerKind) error
}

// dockerRunner is the production Runner. It shells out to `docker compose`
// using the same invocation convention Frank uses everywhere else
// (`--project-directory . -f .frank/compose.yaml`, cwd = project root).
type dockerRunner struct {
	projectDir  string
	composeFile string
}

func newDockerRunner(projectDir, composeFile string) *dockerRunner {
	if composeFile == "" {
		composeFile = ".frank/compose.yaml"
	}
	return &dockerRunner{projectDir: projectDir, composeFile: composeFile}
}

func (d *dockerRunner) Trigger(ctx context.Context, kind TriggerKind) error {
	prefix := []string{"compose", "--project-directory", ".", "-f", d.composeFile}
	var args []string
	switch kind {
	case TriggerQueueRestart:
		args = append(prefix, "exec", "-T", "laravel.test", "php", "artisan", "queue:restart")
	case TriggerScheduleRestart:
		args = append(prefix, "restart", "schedule")
	default:
		return fmt.Errorf("watch: unknown trigger kind %d", kind)
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = d.projectDir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		out := bytes.TrimSpace(buf.Bytes())
		if len(out) > 0 {
			return fmt.Errorf("%w: %s", err, string(out))
		}
		return err
	}
	return nil
}

// Debounce/backoff defaults. Overridable via Config for tests.
const (
	defaultDebounceBase = 250 * time.Millisecond
	defaultDebounceMax  = 5 * time.Second
)

// runDebouncer reads from w.events, coalesces bursts inside the current
// debounce window, then dispatches the two triggers in parallel. Backoff
// doubles on any failed window and resets to base on a clean window.
//
// The first event of a window opens the timer; further events during the
// window are drained and discarded. The timer fires once; dispatch runs;
// the next event starts a fresh window.
func (w *Watcher) runDebouncer(ctx context.Context) {
	base := w.cfg.DebounceBase
	if base <= 0 {
		base = defaultDebounceBase
	}
	max := w.cfg.DebounceMax
	if max <= 0 {
		max = defaultDebounceMax
	}
	if base > max {
		base = max
	}

	window := base

	for {
		// Wait for first event of the next window.
		select {
		case <-ctx.Done():
			return
		case <-w.done:
			return
		case _, ok := <-w.events:
			if !ok {
				return
			}
		}

		// Coalesce everything that arrives within `window`.
		timer := time.NewTimer(window)
	drain:
		for {
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-w.done:
				timer.Stop()
				return
			case _, ok := <-w.events:
				if !ok {
					timer.Stop()
					return
				}
				// Discard: coalesced with the window.
			case <-timer.C:
				break drain
			}
		}

		// Dispatch under a fresh context so a Stop() cancellation aborts
		// in-flight docker calls promptly.
		dispatchCtx, cancel := context.WithCancel(ctx)
		ok := w.dispatch(dispatchCtx)
		cancel()

		if ok {
			window = base
		} else {
			window *= 2
			if window > max {
				window = max
			}
		}
	}
}

// dispatch fires queue:restart and (if enabled) schedule restart in parallel.
// Returns true only if every trigger attempted in this window succeeded.
// Partial failures are logged at WARN per-trigger.
func (w *Watcher) dispatch(ctx context.Context) bool {
	if w.runner == nil {
		// Shouldn't happen — New() installs a default.
		return false
	}

	type result struct {
		kind TriggerKind
		err  error
	}

	kinds := []TriggerKind{TriggerQueueRestart}
	if w.cfg.ScheduleEnabled {
		kinds = append(kinds, TriggerScheduleRestart)
	}

	results := make(chan result, len(kinds))
	var wg sync.WaitGroup
	for _, k := range kinds {
		wg.Add(1)
		go func(kind TriggerKind) {
			defer wg.Done()
			results <- result{kind: kind, err: w.runner.Trigger(ctx, kind)}
		}(k)
	}
	wg.Wait()
	close(results)

	allOK := true
	for r := range results {
		if r.err != nil {
			allOK = false
			fmt.Fprintf(os.Stderr, "frank watch: WARN %s failed: %v\n", triggerLabel(r.kind), r.err)
		}
	}
	return allOK
}

func triggerLabel(k TriggerKind) string {
	switch k {
	case TriggerQueueRestart:
		return "queue:restart"
	case TriggerScheduleRestart:
		return "schedule restart"
	default:
		return "unknown trigger"
	}
}
