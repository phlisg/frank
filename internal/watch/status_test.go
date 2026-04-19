package watch

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadPidfile_Missing(t *testing.T) {
	pid, err := ReadPidfile(filepath.Join(t.TempDir(), "nope.pid"))
	if err != nil {
		t.Fatalf("missing pidfile should not error, got %v", err)
	}
	if pid != 0 {
		t.Fatalf("missing pidfile should return 0, got %d", pid)
	}
}

func TestReadPidfile_Malformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "watch.pid")
	if err := os.WriteFile(path, []byte("not-a-number\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPidfile(path); err == nil {
		t.Fatalf("malformed pidfile should error")
	}
}

func TestWriteAndReadPidfile_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".frank", "watch.pid")
	if err := WritePidfile(path, 4242); err != nil {
		t.Fatalf("WritePidfile: %v", err)
	}
	pid, err := ReadPidfile(path)
	if err != nil {
		t.Fatalf("ReadPidfile: %v", err)
	}
	if pid != 4242 {
		t.Fatalf("round-trip pid mismatch: got %d want 4242", pid)
	}
}

// newCheckerWithFakes wires a StatusChecker around in-memory fakes so
// tests never touch real processes or the filesystem for liveness checks.
// The pidfile on disk is still real — that piece of the contract is what
// we want to exercise.
func newCheckerWithFakes(t *testing.T, laravelRunning bool, pidAlive bool) (*StatusChecker, *fakes) {
	t.Helper()
	root := t.TempDir()
	f := &fakes{pidAlive: pidAlive}

	c := NewStatusChecker(root, func() bool { return laravelRunning })
	c.pidAliveFn = func(_ int) bool { return f.pidAlive }
	c.killFn = func(pid int) error {
		f.killedPID = pid
		f.killCount++
		return nil
	}
	prevRemove := c.removeFn
	c.removeFn = func(p string) error {
		f.removedPaths = append(f.removedPaths, p)
		return prevRemove(p)
	}
	return c, f
}

type fakes struct {
	pidAlive     bool
	killedPID    int
	killCount    int
	removedPaths []string
}

func writePid(t *testing.T, path string, pid int) {
	t.Helper()
	if err := WritePidfile(path, pid); err != nil {
		t.Fatalf("WritePidfile: %v", err)
	}
}

// TestCheck_StoppedWhenPidfileMissing: no pidfile → StatusStopped, no
// side effects.
func TestCheck_StoppedWhenPidfileMissing(t *testing.T) {
	c, f := newCheckerWithFakes(t, true, true)

	got, err := c.Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got.State != StatusStopped {
		t.Errorf("state = %v, want stopped", got.State)
	}
	if f.killCount != 0 {
		t.Errorf("no kill expected, got %d", f.killCount)
	}
}

// TestCheck_StoppedCleansStalePidfile: pidfile references a dead pid →
// StatusStopped AND pidfile is unlinked.
func TestCheck_StoppedCleansStalePidfile(t *testing.T) {
	c, f := newCheckerWithFakes(t, true, false)
	writePid(t, c.PidfilePath, 12345)

	got, err := c.Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got.State != StatusStopped {
		t.Errorf("state = %v, want stopped", got.State)
	}
	if _, err := os.Stat(c.PidfilePath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("stale pidfile should be removed, stat err = %v", err)
	}
	if f.killCount != 0 {
		t.Errorf("dead pid should not be signalled, got kill count %d", f.killCount)
	}
}

// TestCheck_RunningWhenPidAliveAndLaravelUp: happy path → StatusRunning,
// pidfile preserved, no side effects.
func TestCheck_RunningWhenPidAliveAndLaravelUp(t *testing.T) {
	c, f := newCheckerWithFakes(t, true, true)
	writePid(t, c.PidfilePath, 12345)

	got, err := c.Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got.State != StatusRunning {
		t.Errorf("state = %v, want running", got.State)
	}
	if got.PID != 12345 {
		t.Errorf("PID = %d, want 12345", got.PID)
	}
	if got.StartedAt.IsZero() {
		t.Errorf("StartedAt should be set from pidfile mtime")
	}
	if _, err := os.Stat(c.PidfilePath); err != nil {
		t.Errorf("running state must preserve pidfile, stat err = %v", err)
	}
	if f.killCount != 0 {
		t.Errorf("running state must not signal, got kill count %d", f.killCount)
	}
}

// TestCheck_OrphanKillsAndUnlinks: pid alive but laravel down →
// StatusOrphaned, SIGTERM sent, pidfile removed.
func TestCheck_OrphanKillsAndUnlinks(t *testing.T) {
	c, f := newCheckerWithFakes(t, false, true)
	writePid(t, c.PidfilePath, 54321)

	got, err := c.Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got.State != StatusOrphaned {
		t.Errorf("state = %v, want orphaned", got.State)
	}
	if got.PID != 54321 {
		t.Errorf("PID = %d, want 54321", got.PID)
	}
	if f.killCount != 1 || f.killedPID != 54321 {
		t.Errorf("expected one kill of 54321, got count=%d pid=%d", f.killCount, f.killedPID)
	}
	if _, err := os.Stat(c.PidfilePath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("orphan path must unlink pidfile, stat err = %v", err)
	}
}

// TestCheck_MalformedPidfileReturnsErrorAndCleans asserts that an
// unparseable pidfile yields an error AND the pidfile is removed so the
// next run starts clean.
func TestCheck_MalformedPidfileReturnsErrorAndCleans(t *testing.T) {
	c, _ := newCheckerWithFakes(t, true, true)
	if err := os.MkdirAll(filepath.Dir(c.PidfilePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(c.PidfilePath, []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := c.Check()
	if err == nil {
		t.Fatalf("malformed pidfile should surface parse error")
	}
	if got.State != StatusStopped {
		t.Errorf("state = %v, want stopped", got.State)
	}
	if _, err := os.Stat(c.PidfilePath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("malformed pidfile must unlink, stat err = %v", err)
	}
}

// TestPidfilePath_AlwaysDotFrank enforces the .frank/watch.pid convention.
func TestPidfilePath_AlwaysDotFrank(t *testing.T) {
	got := PidfilePath("/some/project")
	want := filepath.Join("/some/project", ".frank", "watch.pid")
	if got != want {
		t.Errorf("PidfilePath = %q, want %q", got, want)
	}
}

// TestStatusState_StringTable locks the --status labels so future edits
// don't silently break user output.
func TestStatusState_StringTable(t *testing.T) {
	cases := []struct {
		in   StatusState
		want string
	}{
		{StatusStopped, "stopped"},
		{StatusRunning, "running"},
		{StatusOrphaned, "orphaned"},
		{StatusState(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.in.String(); got != tc.want {
			t.Errorf("%d.String() = %q, want %q", int(tc.in), got, tc.want)
		}
	}
}
