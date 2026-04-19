// Package watch implements the host-side file watcher that reloads Laravel
// workers on code change.
//
// See docs/superpowers/specs/2026-04-18-workers-schedule-queue-design.md,
// section "Host Watcher" for the design. The watcher observes a narrow set
// of Laravel source paths (see defaultWatchRoots / defaultWatchFiles),
// filters events against a baseline ignore set unioned with the project
// .gitignore, debounces bursts, and dispatches two triggers per window:
// `php artisan queue:restart` inside the laravel.test container and
// `docker compose restart laravel.schedule` for the scheduler.
//
// This file declares the exported surface only. Follow-up tasks own the
// concrete logic:
//   - td-18d17c: walker + fsnotify arming + ignore matching
//   - td-057aa5: debouncer
//   - td-a000b6: trigger dispatcher + backoff
//   - td-4850c4: lifecycle (pidfile, detached child, orphan detection)
package watch

import (
	"context"
	"sync"

	"github.com/fsnotify/fsnotify"
	ignore "github.com/sabhiram/go-gitignore"
)

// Config holds the inputs the watcher needs to arm itself. Fields map to
// the "Host Watcher" section of the design spec.
type Config struct {
	// ProjectRoot is the absolute path to the Laravel project root (the
	// directory containing frank.yaml). Walk anchoring and .gitignore
	// matching are relative to this path — see spec note on anchored
	// patterns.
	ProjectRoot string

	// ScheduleEnabled mirrors workers.schedule from frank.yaml. When false,
	// the dispatcher skips `docker compose restart laravel.schedule` (spec
	// "Trigger dispatch").
	ScheduleEnabled bool

	// QueueCount is the total number of declared queue workers across all
	// pools. Combined with ScheduleEnabled and ad-hoc worker presence, it
	// drives the "Skip conditions" rule in the lifecycle section: no
	// workers → no watcher.
	QueueCount int

	// DockerComposeFile is the path passed via `-f` to every docker compose
	// invocation the dispatcher makes. Must match Frank's invariant of
	// `.frank/compose.yaml` relative to the project directory.
	DockerComposeFile string

	// ExtraPaths is a placeholder for the future `workers.watch.extra_paths`
	// config key (spec "Extending the watch set"). Out of scope for v1;
	// reserved here so the type signature is stable.
	ExtraPaths []string
}

// TriggerKind identifies which reload action a dispatch should perform.
// Used by the future trigger dispatcher (td-a000b6).
type TriggerKind int

const (
	// TriggerQueueRestart runs `php artisan queue:restart` inside
	// laravel.test.
	TriggerQueueRestart TriggerKind = iota

	// TriggerScheduleRestart runs `docker compose restart laravel.schedule`.
	TriggerScheduleRestart
)

// Watcher owns the lifecycle of the fsnotify watch set and the debounced
// dispatch loop. Construct with New; drive with Start / Stop.
type Watcher struct {
	cfg    Config
	fsw    *fsnotify.Watcher
	events chan fsnotify.Event
	done   chan struct{}

	stopOnce sync.Once

	// gitignore is populated by the walker (td-18d17c) from the project
	// .gitignore at arm time. Nil means baseline-only matching.
	gitignore *ignore.GitIgnore
}

// Events exposes the filtered event stream. The debouncer (td-057aa5) is
// the intended consumer — it reads from this channel, coalesces bursts
// within a debounce window, and hands the collapsed trigger to the
// dispatcher (td-a000b6). Only events that pass the classifier filter
// (.php / .env / composer.lock, not ignored) land here.
func (w *Watcher) Events() <-chan fsnotify.Event {
	return w.events
}

// defaultWatchRoots lists the Laravel source directories walked at arm
// time. Matches spec "Watch scope — narrow, not full-project". Consumed by
// the walker (td-18d17c); not wired in this skeleton.
var defaultWatchRoots = []string{
	"app", "bootstrap", "config", "database", "lang",
	"resources/views", "routes",
}

// defaultWatchFiles lists root-level single files watched via their parent
// directory. Event filtering matches by exact basename.
var defaultWatchFiles = []string{".env", "composer.lock"}

// baselineIgnorePatterns is the hardcoded ignore set applied on top of the
// project .gitignore. Only entries that .gitignore can't or typically
// doesn't cover belong here (spec "Baseline (hardcoded, always applied)").
// Matching logic lives with the walker (td-18d17c).
var baselineIgnorePatterns = []string{
	".git/**",
	".frank/**",
	"*.swp",
	"*.swx",
	"*~",
	"4913",
	".DS_Store",
}

// New constructs a Watcher with the given Config. It does NOT arm fsnotify
// watches; walking + Add() calls land in td-18d17c.
func New(cfg Config) (*Watcher, error) {
	return &Watcher{
		cfg:    cfg,
		events: make(chan fsnotify.Event, 128),
		done:   make(chan struct{}),
	}, nil
}

// Start begins watching. Constructs the fsnotify watcher, walks
// defaultWatchRoots (pruning ignored dirs), arms parent-dir watches for
// defaultWatchFiles, and runs a select loop that classifies each event
// and pushes triggering events to w.events for the debouncer to consume.
//
// Blocks until ctx is cancelled or Stop is called.
//
// TODO(td-057aa5, td-a000b6, td-4850c4): debouncer, dispatcher, pidfile.
func (w *Watcher) Start(ctx context.Context) error {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	w.fsw = fsw
	w.gitignore = compileIgnore(w.cfg.ProjectRoot)

	if _, werr := w.armWatches(); werr != nil {
		// Arm errors are non-fatal — log and proceed with whatever
		// watches succeeded. Detailed logging lives in armWatches.
		_ = werr
	}

	for {
		select {
		case <-ctx.Done():
			_ = w.fsw.Close()
			return ctx.Err()
		case <-w.done:
			_ = w.fsw.Close()
			return nil
		case ev, ok := <-w.fsw.Events:
			if !ok {
				return nil
			}
			if _, fire := w.classify(ev); !fire {
				continue
			}
			// Non-blocking send — if the debouncer is slow, we drop
			// the event rather than stall the watch loop. The debouncer
			// only cares that *something* happened in the window.
			select {
			case w.events <- ev:
			default:
			}
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return nil
			}
			// fsnotify errors are informational — log and continue.
			_ = err
		}
	}
}

// Stop gracefully tears down the watcher. Idempotent.
//
// TODO(td-4850c4): coordinate with lifecycle pidfile cleanup.
func (w *Watcher) Stop() error {
	w.stopOnce.Do(func() {
		close(w.done)
	})
	return nil
}
