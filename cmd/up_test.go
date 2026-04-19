package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/phlisg/frank/internal/config"
)

func TestUpCmd_TypedDetachFlag(t *testing.T) {
	// Flag registration: -d and --quick parse as booleans, --build rejected.
	if f := upCmd.Flags().Lookup("detach"); f == nil || f.Shorthand != "d" {
		t.Fatalf("upCmd missing -d/--detach typed flag")
	}
	if f := upCmd.Flags().Lookup("quick"); f == nil {
		t.Fatalf("upCmd missing --quick typed flag")
	}
	if upCmd.Flags().Lookup("build") != nil {
		t.Fatalf("upCmd should not own --build (belongs to docker compose)")
	}
}

func TestUpCmd_UnknownFlagHintsAtDash(t *testing.T) {
	// FlagErrorFunc wraps the raw pflag error with a hint pointing at `--`.
	err := upFlagError(upCmd, errors.New("unknown flag: --build"))
	if err == nil {
		t.Fatalf("upFlagError returned nil")
	}
	if !strings.Contains(err.Error(), "--") || !strings.Contains(err.Error(), "docker compose") {
		t.Errorf("unknown-flag error should hint about `--` and compose, got: %v", err)
	}
}

// buildUpComposeArgs mirrors the composeArgs construction in runUp so the
// behavior can be exercised without booting docker. Keep in sync with runUp.
func buildUpComposeArgs(detach bool, passthrough []string) []string {
	out := append([]string{}, passthrough...)
	if detach {
		out = append([]string{"-d"}, out...)
	}
	return out
}

func TestUpCmd_DetachInjectedIntoComposeArgs(t *testing.T) {
	got := buildUpComposeArgs(true, []string{"--remove-orphans"})
	want := []string{"-d", "--remove-orphans"}
	if !equalSlice(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestUpCmd_NoDetachNoInjection(t *testing.T) {
	got := buildUpComposeArgs(false, []string{"--remove-orphans"})
	want := []string{"--remove-orphans"}
	if !equalSlice(got, want) {
		t.Errorf("got %v, want %v", got, want)
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
