// Package workertop renders a live multi-pane TUI view of a project's
// worker containers (scheduler, declared queue workers, ad-hoc workers).
//
// discover.go resolves the set of panes to render from the frank.yaml
// config and the current docker state — one pane per scheduler (if
// enabled), one pane per declared queue-worker slot, and one pane per
// ad-hoc container labelled frank.worker=adhoc.
package workertop

import (
	"fmt"

	"github.com/phlisg/frank/internal/config"
)

// PaneKind identifies which category a pane belongs to.
type PaneKind int

const (
	// KindSchedule — the laravel.schedule container.
	KindSchedule PaneKind = iota
	// KindQueue — a declared queue-worker slot (laravel.queue.<pool>.<i>).
	KindQueue
	// KindAdhoc — an ad-hoc worker container labelled frank.worker=adhoc.
	KindAdhoc
)

// PaneState describes the current runtime state of a pane's container.
type PaneState int

const (
	// StateMissing — container with the expected name does not exist.
	StateMissing PaneState = iota
	// StateRunning — container exists and is running.
	StateRunning
	// StateExited — container exists but has stopped; see ExitCode.
	StateExited
)

// PaneSpec describes a single pane to render.
type PaneSpec struct {
	// Name is the container name (laravel.schedule, laravel.queue.<pool>.<i>,
	// or the raw ad-hoc container name).
	Name string
	// Kind is the pane category.
	Kind PaneKind
	// Pool is the queue pool name; empty for schedule/adhoc.
	Pool string
	// ContainerID is the docker container id; may be empty when
	// State == StateMissing.
	ContainerID string
	// State is the resolved current state.
	State PaneState
	// ExitCode is valid when State == StateExited.
	ExitCode int
}

// containerInspector is the subset of *docker.Client that discoverWorkers
// depends on. Defined as an interface so tests can mock it.
type containerInspector interface {
	// InspectContainer returns (status, exitCode, id) for the named
	// container. status is one of docker's state strings ("running",
	// "exited", "created", etc.) or "" when the container does not exist.
	// When the container doesn't exist, the implementation must return
	// ("", 0, "", nil) — no error.
	InspectContainer(name string) (status string, exitCode int, id string, err error)
	// AdhocWorkerNames returns container names labelled frank.worker=adhoc
	// for the given compose project, including stopped ones.
	AdhocWorkerNames(projectName string) ([]string, error)
}

// discoverWorkers enumerates every pane the TUI should render for the
// current configuration and docker state.
//
// Order: schedule (if enabled), then declared queue workers in config
// order (pool-major, slot-minor), then ad-hoc workers in the order
// returned by AdhocWorkerNames.
func discoverWorkers(cfg *config.Config, projectName string, d containerInspector) ([]PaneSpec, error) {
	if cfg == nil {
		return nil, fmt.Errorf("discoverWorkers: nil config")
	}
	if d == nil {
		return nil, fmt.Errorf("discoverWorkers: nil inspector")
	}

	var specs []PaneSpec

	if cfg.Workers.Schedule {
		spec := PaneSpec{
			Name: "laravel.schedule",
			Kind: KindSchedule,
		}
		if err := resolveState(d, &spec); err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}

	for _, pool := range cfg.Workers.Queue {
		for i := 1; i <= pool.Count; i++ {
			spec := PaneSpec{
				Name: fmt.Sprintf("laravel.queue.%s.%d", pool.Name, i),
				Kind: KindQueue,
				Pool: pool.Name,
			}
			if err := resolveState(d, &spec); err != nil {
				return nil, err
			}
			specs = append(specs, spec)
		}
	}

	adhoc, err := d.AdhocWorkerNames(projectName)
	if err != nil {
		return nil, fmt.Errorf("discoverWorkers: list ad-hoc workers: %w", err)
	}
	for _, name := range adhoc {
		spec := PaneSpec{
			Name: name,
			Kind: KindAdhoc,
		}
		if err := resolveState(d, &spec); err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}

	return specs, nil
}

// resolveState inspects the container named spec.Name and fills in
// ContainerID, State, and (when exited) ExitCode on spec.
func resolveState(d containerInspector, spec *PaneSpec) error {
	status, exitCode, id, err := d.InspectContainer(spec.Name)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", spec.Name, err)
	}
	spec.ContainerID = id
	switch status {
	case "":
		spec.State = StateMissing
	case "running":
		spec.State = StateRunning
	default:
		// exited, dead, created, paused, restarting — treat anything
		// non-running as exited. ExitCode is only meaningful for true
		// exits but carrying docker's reported code through is fine for
		// the title bar.
		spec.State = StateExited
		spec.ExitCode = exitCode
	}
	return nil
}
