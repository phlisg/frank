package cmd

import (
	"testing"

	"github.com/phlisg/frank/internal/config"
)

func TestDetachedMode(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"empty", nil, false},
		{"just build flag", []string{"--build"}, false},
		{"-d short", []string{"-d"}, true},
		{"--detach long", []string{"--detach"}, true},
		{"--detach=true", []string{"--detach=true"}, true},
		{"--detach=false", []string{"--detach=false"}, false},
		{"--detach=0", []string{"--detach=0"}, false},
		{"-d with other flags", []string{"--build", "-d", "--force-recreate"}, true},
		{"detached substring not enough", []string{"--detached"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := detachedMode(tc.args); got != tc.want {
				t.Errorf("detachedMode(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestShouldRunWatcher_ScheduleEnabled(t *testing.T) {
	cfg := &config.Config{}
	cfg.Workers.Schedule = true
	if !shouldRunWatcher(cfg, nil, t.TempDir()) {
		t.Errorf("schedule=true should request watcher")
	}
}

func TestShouldRunWatcher_QueueDeclared(t *testing.T) {
	cfg := &config.Config{}
	cfg.Workers.Queue = []config.QueuePool{{Count: 1}}
	if !shouldRunWatcher(cfg, nil, t.TempDir()) {
		t.Errorf("queue count > 0 should request watcher")
	}
}

func TestShouldRunWatcher_NoWorkersNoAdhoc(t *testing.T) {
	cfg := &config.Config{}
	if shouldRunWatcher(cfg, nil, t.TempDir()) {
		t.Errorf("no workers + no adhoc should skip watcher")
	}
}

func TestShouldRunWatcher_NilCfgNilClient(t *testing.T) {
	if shouldRunWatcher(nil, nil, t.TempDir()) {
		t.Errorf("no config and no client → no watcher")
	}
}
