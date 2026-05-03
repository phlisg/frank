package workertop

import (
	"errors"
	"reflect"
	"testing"

	"github.com/phlisg/frank/internal/config"
)

// fakeInspector is a table-driven stand-in for the docker client.
//
// containers maps container name → (status, exitCode, id). A missing
// entry signals "no such container" (status "", no error).
// adhoc is the slice returned by AdhocWorkerNames.
type fakeInspector struct {
	containers map[string]fakeContainer
	adhoc      []string
	adhocErr   error
	inspectErr map[string]error // optional per-name inspect errors
}

type fakeContainer struct {
	status   string
	exitCode int
	id       string
}

func (f *fakeInspector) InspectContainer(name string) (string, int, string, error) {
	if err := f.inspectErr[name]; err != nil {
		return "", 0, "", err
	}
	c, ok := f.containers[name]
	if !ok {
		return "", 0, "", nil
	}
	return c.status, c.exitCode, c.id, nil
}

func (f *fakeInspector) AdhocWorkerNames(projectName string) ([]string, error) {
	if f.adhocErr != nil {
		return nil, f.adhocErr
	}
	return f.adhoc, nil
}

func TestDiscoverWorkers(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.Config
		insp    *fakeInspector
		want    []PaneSpec
		wantErr bool
	}{
		{
			name: "schedule only, running",
			cfg: &config.Config{
				Workers: config.Workers{Schedule: true},
			},
			insp: &fakeInspector{
				containers: map[string]fakeContainer{
					"frank-test-schedule-1": {status: "running", id: "sched-id"},
				},
			},
			want: []PaneSpec{
				{
					Name:        "schedule",
					Kind:        KindSchedule,
					ContainerID: "sched-id",
					State:       StateRunning,
				},
			},
		},
		{
			name: "schedule plus one pool count=3 — 4 panes",
			cfg: &config.Config{
				Workers: config.Workers{
					Schedule: true,
					Queue: []config.QueuePool{
						{Name: "default", Count: 3},
					},
				},
			},
			insp: &fakeInspector{
				containers: map[string]fakeContainer{
					"frank-test-schedule-1":        {status: "running", id: "s1"},
					"frank-test-queue.default.1-1": {status: "running", id: "q1"},
					"frank-test-queue.default.2-1": {status: "running", id: "q2"},
					"frank-test-queue.default.3-1": {status: "running", id: "q3"},
				},
			},
			want: []PaneSpec{
				{Name: "schedule", Kind: KindSchedule, ContainerID: "s1", State: StateRunning},
				{Name: "queue.default.1", Kind: KindQueue, Pool: "default", ContainerID: "q1", State: StateRunning},
				{Name: "queue.default.2", Kind: KindQueue, Pool: "default", ContainerID: "q2", State: StateRunning},
				{Name: "queue.default.3", Kind: KindQueue, Pool: "default", ContainerID: "q3", State: StateRunning},
			},
		},
		{
			name: "schedule disabled, two pools (2 + 1) — 3 panes",
			cfg: &config.Config{
				Workers: config.Workers{
					Schedule: false,
					Queue: []config.QueuePool{
						{Name: "default", Count: 2},
						{Name: "emails", Count: 1},
					},
				},
			},
			insp: &fakeInspector{
				containers: map[string]fakeContainer{
					"frank-test-queue.default.1-1": {status: "running", id: "d1"},
					"frank-test-queue.default.2-1": {status: "running", id: "d2"},
					"frank-test-queue.emails.1-1":  {status: "running", id: "e1"},
				},
			},
			want: []PaneSpec{
				{Name: "queue.default.1", Kind: KindQueue, Pool: "default", ContainerID: "d1", State: StateRunning},
				{Name: "queue.default.2", Kind: KindQueue, Pool: "default", ContainerID: "d2", State: StateRunning},
				{Name: "queue.emails.1", Kind: KindQueue, Pool: "emails", ContainerID: "e1", State: StateRunning},
			},
		},
		{
			name: "all three kinds mixed",
			cfg: &config.Config{
				Workers: config.Workers{
					Schedule: true,
					Queue: []config.QueuePool{
						{Name: "default", Count: 1},
					},
				},
			},
			insp: &fakeInspector{
				containers: map[string]fakeContainer{
					"frank-test-schedule-1":        {status: "running", id: "sched"},
					"frank-test-queue.default.1-1": {status: "running", id: "q1"},
					"adhoc-debug":                  {status: "running", id: "adhoc1"},
				},
				adhoc: []string{"adhoc-debug"},
			},
			want: []PaneSpec{
				{Name: "schedule", Kind: KindSchedule, ContainerID: "sched", State: StateRunning},
				{Name: "queue.default.1", Kind: KindQueue, Pool: "default", ContainerID: "q1", State: StateRunning},
				{Name: "adhoc-debug", Kind: KindAdhoc, ContainerID: "adhoc1", State: StateRunning},
			},
		},
		{
			name: "declared worker missing container",
			cfg: &config.Config{
				Workers: config.Workers{
					Queue: []config.QueuePool{
						{Name: "default", Count: 2},
					},
				},
			},
			insp: &fakeInspector{
				containers: map[string]fakeContainer{
					"frank-test-queue.default.1-1": {status: "running", id: "d1"},
					// laravel.queue.default.2 missing
				},
			},
			want: []PaneSpec{
				{Name: "queue.default.1", Kind: KindQueue, Pool: "default", ContainerID: "d1", State: StateRunning},
				{Name: "queue.default.2", Kind: KindQueue, Pool: "default", State: StateMissing},
			},
		},
		{
			name: "container exited with code 1",
			cfg: &config.Config{
				Workers: config.Workers{
					Queue: []config.QueuePool{
						{Name: "default", Count: 1},
					},
				},
			},
			insp: &fakeInspector{
				containers: map[string]fakeContainer{
					"frank-test-queue.default.1-1": {status: "exited", exitCode: 1, id: "dead1"},
				},
			},
			want: []PaneSpec{
				{Name: "queue.default.1", Kind: KindQueue, Pool: "default", ContainerID: "dead1", State: StateExited, ExitCode: 1},
			},
		},
		{
			name: "empty config — no panes",
			cfg:  &config.Config{},
			insp: &fakeInspector{},
			want: nil,
		},
		{
			name: "adhoc listing error propagates",
			cfg: &config.Config{
				Workers: config.Workers{Schedule: true},
			},
			insp: &fakeInspector{
				containers: map[string]fakeContainer{
					"frank-test-schedule-1": {status: "running", id: "s"},
				},
				adhocErr: errors.New("docker ps failed"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := discoverWorkers(tt.cfg, "frank-test", tt.insp)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("pane specs mismatch\n got: %#v\nwant: %#v", got, tt.want)
			}
		})
	}
}

func TestDiscoverWorkers_NilArgs(t *testing.T) {
	if _, err := discoverWorkers(nil, "p", &fakeInspector{}); err == nil {
		t.Errorf("expected error for nil config")
	}
	if _, err := discoverWorkers(&config.Config{}, "p", nil); err == nil {
		t.Errorf("expected error for nil inspector")
	}
}
