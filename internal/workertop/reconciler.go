package workertop

import (
	"context"
	"time"
)

// EventType discriminates add vs remove events.
type EventType int

const (
	// EventAdd — a new ad-hoc worker container appeared.
	EventAdd EventType = iota
	// EventRemove — a previously-known ad-hoc worker disappeared.
	EventRemove
)

// ReconcileEvent is emitted by the Reconciler on each diff transition.
//
// For EventAdd, Spec is fully populated (Name, Kind=KindAdhoc, ContainerID,
// State=StateRunning). For EventRemove, only Spec.Name is set — the consumer
// looks up its existing pane by name to tear it down.
type ReconcileEvent struct {
	Type EventType
	Spec PaneSpec
}

// AdhocContainer is the minimal shape returned by the lister for each
// frank.worker=adhoc container.
type AdhocContainer struct {
	ID   string
	Name string
}

// adhocLister is the docker-facing boundary. Implementations poll
// `docker ps --filter label=frank.worker=adhoc`; tests supply a stub.
type adhocLister interface {
	ListAdhocWorkers() ([]AdhocContainer, error)
}

// Reconciler polls the adhocLister on a fixed interval and emits add/remove
// events as the set of ad-hoc workers churns.
type Reconciler struct {
	lister   adhocLister
	interval time.Duration
	events   chan ReconcileEvent
	current  map[string]struct{}
}

// NewReconciler builds a Reconciler.
//
// initial is the set of PaneSpecs the TopModel already has from discovery.
// Only KindAdhoc entries are relevant — their names seed the "known"
// snapshot so the first tick does not re-emit EventAdd for containers that
// were already on screen. Pass nil when there is no pre-existing set.
func NewReconciler(lister adhocLister, interval time.Duration, initial []PaneSpec) *Reconciler {
	current := make(map[string]struct{})
	for _, s := range initial {
		if s.Kind == KindAdhoc {
			current[s.Name] = struct{}{}
		}
	}
	return &Reconciler{
		lister:   lister,
		interval: interval,
		events:   make(chan ReconcileEvent, 16),
		current:  current,
	}
}

// Events returns the receive-only channel of ReconcileEvents. The channel is
// closed when Run's context is cancelled.
func (r *Reconciler) Events() <-chan ReconcileEvent {
	return r.events
}

// Run ticks every interval, diffs the latest ad-hoc set against the prior
// snapshot, and emits events. List errors skip the tick silently. The
// events channel is closed on ctx.Done before returning.
func (r *Reconciler) Run(ctx context.Context) {
	defer close(r.events)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			latest, err := r.lister.ListAdhocWorkers()
			if err != nil {
				continue
			}
			added, removed := diffAdhoc(r.current, latest)
			for _, c := range added {
				select {
				case <-ctx.Done():
					return
				case r.events <- ReconcileEvent{
					Type: EventAdd,
					Spec: PaneSpec{
						Name:        c.Name,
						Kind:        KindAdhoc,
						ContainerID: c.ID,
						State:       StateRunning,
					},
				}:
				}
				r.current[c.Name] = struct{}{}
			}
			for _, name := range removed {
				select {
				case <-ctx.Done():
					return
				case r.events <- ReconcileEvent{
					Type: EventRemove,
					Spec: PaneSpec{Name: name},
				}:
				}
				delete(r.current, name)
			}
		}
	}
}

// diffAdhoc computes (added, removed) for the transition from the current
// snapshot to latest. It does not mutate current — callers update their
// snapshot themselves based on the returned slices.
//
// Order: added follows the order of latest; removed is emitted in the
// iteration order of the current map (unstable — callers that care about
// determinism should not rely on it).
func diffAdhoc(current map[string]struct{}, latest []AdhocContainer) (added []AdhocContainer, removed []string) {
	latestSet := make(map[string]struct{}, len(latest))
	for _, c := range latest {
		latestSet[c.Name] = struct{}{}
		if _, ok := current[c.Name]; !ok {
			added = append(added, c)
		}
	}
	for name := range current {
		if _, ok := latestSet[name]; !ok {
			removed = append(removed, name)
		}
	}
	return added, removed
}
