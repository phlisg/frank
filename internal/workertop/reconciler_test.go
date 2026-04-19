package workertop

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"
)

// fakeLister is a scripted adhocLister. Each call to ListAdhocWorkers pops
// the next entry from ticks; once exhausted it returns the final entry
// forever. If err is non-nil at a given position, that tick returns the
// error instead of a list.
type fakeLister struct {
	mu    sync.Mutex
	ticks []fakeTick
	calls int
}

type fakeTick struct {
	list []AdhocContainer
	err  error
}

func (f *fakeLister) ListAdhocWorkers() ([]AdhocContainer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.calls
	if i >= len(f.ticks) {
		i = len(f.ticks) - 1
	}
	f.calls++
	t := f.ticks[i]
	return t.list, t.err
}

func (f *fakeLister) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// collectEvents drains up to want events from the channel, or fails the
// test if they don't arrive within timeout.
func collectEvents(t *testing.T, ch <-chan ReconcileEvent, want int, timeout time.Duration) []ReconcileEvent {
	t.Helper()
	var got []ReconcileEvent
	deadline := time.After(timeout)
	for len(got) < want {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatalf("events channel closed early; got %d/%d events: %+v", len(got), want, got)
			}
			got = append(got, ev)
		case <-deadline:
			t.Fatalf("timed out waiting for %d events; got %d: %+v", want, len(got), got)
		}
	}
	return got
}

func TestDiffAdhoc(t *testing.T) {
	tests := []struct {
		name        string
		current     []string
		latest      []AdhocContainer
		wantAdded   []string
		wantRemoved []string
	}{
		{
			name:    "no change",
			current: []string{"a", "b"},
			latest: []AdhocContainer{
				{ID: "1", Name: "a"},
				{ID: "2", Name: "b"},
			},
			wantAdded:   nil,
			wantRemoved: nil,
		},
		{
			name:    "one add",
			current: []string{"a"},
			latest: []AdhocContainer{
				{ID: "1", Name: "a"},
				{ID: "2", Name: "b"},
			},
			wantAdded:   []string{"b"},
			wantRemoved: nil,
		},
		{
			name:    "one remove",
			current: []string{"a", "b"},
			latest: []AdhocContainer{
				{ID: "1", Name: "a"},
			},
			wantAdded:   nil,
			wantRemoved: []string{"b"},
		},
		{
			name:    "churn: one in one out",
			current: []string{"a", "b"},
			latest: []AdhocContainer{
				{ID: "1", Name: "a"},
				{ID: "3", Name: "c"},
			},
			wantAdded:   []string{"c"},
			wantRemoved: []string{"b"},
		},
		{
			name:        "empty to populated",
			current:     nil,
			latest:      []AdhocContainer{{ID: "1", Name: "a"}, {ID: "2", Name: "b"}},
			wantAdded:   []string{"a", "b"},
			wantRemoved: nil,
		},
		{
			name:        "populated to empty",
			current:     []string{"a", "b"},
			latest:      nil,
			wantAdded:   nil,
			wantRemoved: []string{"a", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cur := make(map[string]struct{}, len(tt.current))
			for _, n := range tt.current {
				cur[n] = struct{}{}
			}
			added, removed := diffAdhoc(cur, tt.latest)

			gotAdded := make([]string, 0, len(added))
			for _, a := range added {
				gotAdded = append(gotAdded, a.Name)
			}
			sort.Strings(gotAdded)
			sort.Strings(removed)
			wantAdded := append([]string(nil), tt.wantAdded...)
			wantRemoved := append([]string(nil), tt.wantRemoved...)
			sort.Strings(wantAdded)
			sort.Strings(wantRemoved)

			if !stringSliceEqual(gotAdded, wantAdded) {
				t.Errorf("added: got %v, want %v", gotAdded, wantAdded)
			}
			if !stringSliceEqual(removed, wantRemoved) {
				t.Errorf("removed: got %v, want %v", removed, wantRemoved)
			}
		})
	}
}

func stringSliceEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestReconciler_NoPreexisting(t *testing.T) {
	lister := &fakeLister{
		ticks: []fakeTick{
			{list: []AdhocContainer{{ID: "id-a", Name: "A"}, {ID: "id-b", Name: "B"}}},
		},
	}
	r := NewReconciler(lister, 10*time.Millisecond, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go r.Run(ctx)

	events := collectEvents(t, r.Events(), 2, 500*time.Millisecond)

	got := map[string]ReconcileEvent{}
	for _, ev := range events {
		if ev.Type != EventAdd {
			t.Errorf("unexpected event type %v for %s", ev.Type, ev.Spec.Name)
		}
		got[ev.Spec.Name] = ev
	}
	if len(got) != 2 {
		t.Fatalf("want 2 distinct adds, got %d", len(got))
	}
	for _, name := range []string{"A", "B"} {
		ev, ok := got[name]
		if !ok {
			t.Errorf("missing EventAdd for %s", name)
			continue
		}
		if ev.Spec.Kind != KindAdhoc {
			t.Errorf("%s: kind = %v, want KindAdhoc", name, ev.Spec.Kind)
		}
		if ev.Spec.State != StateRunning {
			t.Errorf("%s: state = %v, want StateRunning", name, ev.Spec.State)
		}
		if ev.Spec.ContainerID == "" {
			t.Errorf("%s: empty ContainerID", name)
		}
	}
}

func TestReconciler_Preexisting(t *testing.T) {
	initial := []PaneSpec{
		{Name: "A", Kind: KindAdhoc, ContainerID: "id-a", State: StateRunning},
		// Non-adhoc entries must be ignored by seeding.
		{Name: "laravel.schedule", Kind: KindSchedule},
	}
	lister := &fakeLister{
		ticks: []fakeTick{
			{list: []AdhocContainer{{ID: "id-a", Name: "A"}, {ID: "id-b", Name: "B"}}},
		},
	}
	r := NewReconciler(lister, 10*time.Millisecond, initial)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go r.Run(ctx)

	events := collectEvents(t, r.Events(), 1, 500*time.Millisecond)

	if len(events) != 1 {
		t.Fatalf("want exactly 1 event, got %d: %+v", len(events), events)
	}
	ev := events[0]
	if ev.Type != EventAdd {
		t.Errorf("type = %v, want EventAdd", ev.Type)
	}
	if ev.Spec.Name != "B" {
		t.Errorf("name = %q, want %q (A was preseeded)", ev.Spec.Name, "B")
	}

	// Verify no spurious follow-up events while the fleet stays stable.
	select {
	case ev := <-r.Events():
		t.Errorf("unexpected extra event: %+v", ev)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestReconciler_Churn(t *testing.T) {
	lister := &fakeLister{
		ticks: []fakeTick{
			{list: []AdhocContainer{{ID: "id-a", Name: "A"}, {ID: "id-b", Name: "B"}}},
			{list: []AdhocContainer{{ID: "id-b", Name: "B"}, {ID: "id-c", Name: "C"}}},
		},
	}
	r := NewReconciler(lister, 10*time.Millisecond, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go r.Run(ctx)

	// First tick: adds A and B.
	first := collectEvents(t, r.Events(), 2, 500*time.Millisecond)
	firstNames := map[string]bool{}
	for _, ev := range first {
		if ev.Type != EventAdd {
			t.Errorf("first tick: unexpected type %v for %s", ev.Type, ev.Spec.Name)
		}
		firstNames[ev.Spec.Name] = true
	}
	if !firstNames["A"] || !firstNames["B"] {
		t.Fatalf("first tick: expected adds {A, B}, got %v", firstNames)
	}

	// Second tick: remove A, add C.
	second := collectEvents(t, r.Events(), 2, 500*time.Millisecond)
	var sawRemoveA, sawAddC bool
	for _, ev := range second {
		switch {
		case ev.Type == EventRemove && ev.Spec.Name == "A":
			sawRemoveA = true
		case ev.Type == EventAdd && ev.Spec.Name == "C":
			sawAddC = true
		default:
			t.Errorf("second tick: unexpected event %+v", ev)
		}
	}
	if !sawRemoveA {
		t.Error("second tick: missing EventRemove for A")
	}
	if !sawAddC {
		t.Error("second tick: missing EventAdd for C")
	}
}

func TestReconciler_ListError(t *testing.T) {
	lister := &fakeLister{
		ticks: []fakeTick{
			{err: errors.New("docker not reachable")},
			{err: errors.New("still broken")},
			{list: []AdhocContainer{{ID: "id-a", Name: "A"}}},
		},
	}
	r := NewReconciler(lister, 10*time.Millisecond, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go r.Run(ctx)

	// An add should come through eventually despite the two error ticks.
	events := collectEvents(t, r.Events(), 1, 1*time.Second)
	if events[0].Type != EventAdd || events[0].Spec.Name != "A" {
		t.Fatalf("want EventAdd for A, got %+v", events[0])
	}
	if lister.callCount() < 3 {
		t.Errorf("want >= 3 list calls (two errors + success), got %d", lister.callCount())
	}
}

func TestReconciler_Cancel(t *testing.T) {
	lister := &fakeLister{
		ticks: []fakeTick{
			{list: nil},
		},
	}
	r := NewReconciler(lister, 10*time.Millisecond, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Run(ctx)
		close(done)
	}()

	// Give the ticker at least one pass so we know the loop is live.
	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run did not return after ctx cancel")
	}

	// Events channel must be closed.
	select {
	case _, ok := <-r.Events():
		if ok {
			t.Error("events channel yielded a value after cancel; expected closed")
		}
	default:
		// Channel may be closed with no pending value — drain and retry.
		if _, ok := <-r.Events(); ok {
			t.Error("events channel yielded a value after cancel; expected closed")
		}
	}
}
