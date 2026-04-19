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

	// gitignore is populated by the walker (td-18d17c) from the project
	// .gitignore at arm time. Nil means baseline-only matching.
	gitignore *ignore.GitIgnore
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

// Start begins watching. Blocks until ctx is cancelled or Stop is called.
//
// TODO(td-18d17c, td-057aa5, td-a000b6): walker + debouncer + dispatcher.
func (w *Watcher) Start(ctx context.Context) error {
	return nil
}

// Stop gracefully tears down the watcher.
//
// TODO(td-18d17c, td-4850c4): close fsnotify watcher, drain channels,
// coordinate with lifecycle pidfile cleanup.
func (w *Watcher) Stop() error {
	return nil
}
