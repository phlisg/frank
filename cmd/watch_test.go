package cmd

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/watch"
)

func TestFormatUptime_Ranges(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{5 * time.Second, "5s"},
		{59 * time.Second, "59s"},
		{90 * time.Second, "1m30s"},
		{59*time.Minute + 59*time.Second, "59m59s"},
		{90 * time.Minute, "1h30m"},
		{23 * time.Hour, "23h00m"},
		{25 * time.Hour, "1d01h"},
		{49 * time.Hour, "2d01h"},
	}
	for _, tc := range cases {
		if got := formatUptime(tc.in); got != tc.want {
			t.Errorf("formatUptime(%s) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTotalQueueCount_SumsPositiveCounts(t *testing.T) {
	cfg := &config.Config{}
	cfg.Workers.Queue = []config.QueuePool{
		{Count: 2},
		{Count: 3},
		{Count: 0}, // ignored
	}
	if got := totalQueueCount(cfg); got != 5 {
		t.Errorf("totalQueueCount = %d, want 5", got)
	}
}

// TestRunWatchStop_StaleDeadPidCleansUp: seeded pidfile with a pid that
// is definitely dead → syscall.Kill returns ESRCH → runWatchStop removes
// pidfile and returns nil.
func TestRunWatchStop_StaleDeadPidCleansUp(t *testing.T) {
	root := t.TempDir()
	const deadPid = 2_147_483_600
	if err := watch.WritePidfile(watch.PidfilePath(root), deadPid); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := runWatchStop(root); err != nil {
		t.Fatalf("runWatchStop: %v", err)
	}
	if _, err := os.Stat(watch.PidfilePath(root)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("pidfile should be removed, stat err = %v", err)
	}
}

// TestRunWatchStop_NoPidfile: no pidfile → return nil (nothing to stop).
func TestRunWatchStop_NoPidfile(t *testing.T) {
	root := t.TempDir()
	if err := runWatchStop(root); err != nil {
		t.Fatalf("runWatchStop on empty dir: %v", err)
	}
}

// TestRunWatchStop_MalformedPidfile: garbage content → error returned
// AND pidfile removed so the next run starts clean.
func TestRunWatchStop_MalformedPidfile(t *testing.T) {
	root := t.TempDir()
	path := watch.PidfilePath(root)
	if err := os.MkdirAll(rootDotFrank(root), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runWatchStop(root); err == nil {
		t.Fatalf("expected error for malformed pidfile")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("malformed pidfile should be unlinked, stat err = %v", err)
	}
}

func rootDotFrank(root string) string {
	return root + "/.frank"
}
