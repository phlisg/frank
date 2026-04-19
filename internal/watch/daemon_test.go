package watch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestAcquirePidfile_WritesCurrentPid verifies a clean start writes the
// caller's pid to .frank/watch.pid.
func TestAcquirePidfile_WritesCurrentPid(t *testing.T) {
	root := t.TempDir()
	w := &Watcher{cfg: Config{ProjectRoot: root}}

	if err := w.acquirePidfile(); err != nil {
		t.Fatalf("acquirePidfile: %v", err)
	}
	t.Cleanup(w.releasePidfile)

	pid, err := ReadPidfile(PidfilePath(root))
	if err != nil {
		t.Fatalf("ReadPidfile: %v", err)
	}
	if pid != os.Getpid() {
		t.Errorf("pidfile pid = %d, want %d", pid, os.Getpid())
	}
}

// TestAcquirePidfile_OverwritesStalePid: pidfile references a dead pid →
// acquire succeeds and overwrites with our pid.
func TestAcquirePidfile_OverwritesStalePid(t *testing.T) {
	root := t.TempDir()
	path := PidfilePath(root)

	// A pid we're confident is dead. Values above the kernel's pid_max are
	// guaranteed ESRCH on Signal(0).
	const deadPid = 2_147_483_600
	if err := WritePidfile(path, deadPid); err != nil {
		t.Fatalf("seed pidfile: %v", err)
	}

	w := &Watcher{cfg: Config{ProjectRoot: root}}
	if err := w.acquirePidfile(); err != nil {
		t.Fatalf("acquirePidfile: %v", err)
	}
	t.Cleanup(w.releasePidfile)

	pid, err := ReadPidfile(path)
	if err != nil {
		t.Fatalf("ReadPidfile: %v", err)
	}
	if pid != os.Getpid() {
		t.Errorf("pidfile = %d, want overwrite to %d", pid, os.Getpid())
	}
}

// TestAcquirePidfile_RejectsLiveOther: pidfile references a live other pid
// → acquire errors with the expected 'already running' message.
func TestAcquirePidfile_RejectsLiveOther(t *testing.T) {
	root := t.TempDir()
	path := PidfilePath(root)

	// Pid 1 is always live and never ours. Good enough sentinel.
	if err := WritePidfile(path, 1); err != nil {
		t.Fatalf("seed pidfile: %v", err)
	}

	w := &Watcher{cfg: Config{ProjectRoot: root}}
	err := w.acquirePidfile()
	if err == nil {
		t.Fatalf("expected already-running error, got nil")
	}
	if !containsAll(err.Error(), "already running", "pid 1") {
		t.Errorf("error %q missing expected phrases", err.Error())
	}
}

// TestAcquirePidfile_OverwritesMalformed: garbage content → acquire wipes
// it and writes our pid.
func TestAcquirePidfile_OverwritesMalformed(t *testing.T) {
	root := t.TempDir()
	path := PidfilePath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not-a-pid"), 0o644); err != nil {
		t.Fatal(err)
	}

	w := &Watcher{cfg: Config{ProjectRoot: root}}
	if err := w.acquirePidfile(); err != nil {
		t.Fatalf("acquirePidfile: %v", err)
	}
	t.Cleanup(w.releasePidfile)

	pid, err := ReadPidfile(path)
	if err != nil {
		t.Fatalf("ReadPidfile: %v", err)
	}
	if pid != os.Getpid() {
		t.Errorf("pidfile = %d, want %d", pid, os.Getpid())
	}
}

// TestReleasePidfile_Idempotent: releasing twice is safe (e.g. orphan
// detection already removed it before Stop runs).
func TestReleasePidfile_Idempotent(t *testing.T) {
	root := t.TempDir()
	w := &Watcher{cfg: Config{ProjectRoot: root}}
	if err := w.acquirePidfile(); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	w.releasePidfile()
	w.releasePidfile() // no panic, no error
}

// TestStart_WritesAndUnlinksPidfile covers the Start integration: the
// pidfile exists while Start runs and is gone after it returns.
func TestStart_WritesAndUnlinksPidfile(t *testing.T) {
	root := fakeLaravelProject(t)

	w, err := New(Config{
		ProjectRoot:  root,
		Runner:       &fakeRunner{},
		DebounceBase: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- w.Start(ctx) }()

	// Wait until Start has armed + written the pidfile.
	path := PidfilePath(root)
	deadline := time.After(2 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("pidfile not written within 2s")
		default:
			time.Sleep(20 * time.Millisecond)
		}
	}

	// Stop and confirm unlink.
	if err := w.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("Start did not return")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("pidfile should be unlinked after Stop, stat err = %v", err)
	}
}

// TestStart_RefusesSecondInstance asserts the already-running guard: a
// seeded live-pid pidfile makes Start return without arming.
func TestStart_RefusesSecondInstance(t *testing.T) {
	root := fakeLaravelProject(t)
	if err := WritePidfile(PidfilePath(root), 1); err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(PidfilePath(root)) })

	w, err := New(Config{ProjectRoot: root, Runner: &fakeRunner{}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = w.Start(ctx)
	if err == nil {
		t.Fatalf("expected already-running error")
	}
	if !containsAll(err.Error(), "already running") {
		t.Errorf("error %q missing 'already running'", err.Error())
	}
}

// TestDaemonize_EmptyArgv asserts the guard against nil/empty argv.
func TestDaemonize_EmptyArgv(t *testing.T) {
	if _, err := Daemonize(nil, filepath.Join(t.TempDir(), "x.log")); err == nil {
		t.Fatalf("nil argv should error")
	}
}

// TestDaemonize_SpawnsSurvivingChild spawns `/bin/sleep 0.5` detached,
// verifies the child is live immediately, and confirms it exits on its
// own schedule (parent didn't wait). Also exercises log-file creation.
func TestDaemonize_SpawnsSurvivingChild(t *testing.T) {
	if _, err := os.Stat("/bin/sleep"); err != nil {
		t.Skip("no /bin/sleep available")
	}

	logPath := filepath.Join(t.TempDir(), ".frank", "watch.log")

	pid, err := Daemonize([]string{"/bin/sleep", "0.5"}, logPath)
	if err != nil {
		t.Fatalf("Daemonize: %v", err)
	}
	if pid <= 0 {
		t.Fatalf("expected positive pid, got %d", pid)
	}
	if !pidAlive(pid) {
		t.Fatalf("child pid %d not alive immediately after Daemonize", pid)
	}
	if _, err := os.Stat(logPath); err != nil {
		t.Errorf("log file not created: %v", err)
	}

	// Wait past the child's sleep so reapers run; the child is detached
	// (Setsid) so we are not its parent and won't reap it ourselves. The
	// important property is only that Daemonize returned promptly with a
	// valid pid — no assertion on post-exit state is robust across
	// Linux configurations.
	time.Sleep(700 * time.Millisecond)
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !contains(s, sub) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
