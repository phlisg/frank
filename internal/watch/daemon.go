package watch

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// LogfileName is the basename of the watcher log, relative to .frank/.
const LogfileName = "watch.log"

// LogfilePath returns the absolute watch log path for the given project root.
func LogfilePath(projectRoot string) string {
	return filepath.Join(projectRoot, ".frank", LogfileName)
}

// acquirePidfile writes the current process's pid to .frank/watch.pid,
// refusing to start if another live watcher already owns it.
//
// Stale pidfile handling:
//   - pidfile missing / empty / malformed → overwrite.
//   - pidfile references a dead pid        → overwrite.
//   - pidfile references a live pid        → error (already running).
func (w *Watcher) acquirePidfile() error {
	path := PidfilePath(w.cfg.ProjectRoot)
	existing, err := ReadPidfile(path)
	if err != nil {
		// Malformed content: wipe and proceed.
		_ = os.Remove(path)
	} else if existing != 0 && pidAlive(existing) && existing != os.Getpid() {
		return fmt.Errorf("watch: already running (pid %d); run `frank watch --stop` first", existing)
	}
	return WritePidfile(path, os.Getpid())
}

// releasePidfile unlinks the pidfile. Safe to call if it's already gone
// (the --stop path or an orphan-detection cleanup may have won the race).
func (w *Watcher) releasePidfile() {
	_ = os.Remove(PidfilePath(w.cfg.ProjectRoot))
}

// Daemonize spawns argv as a detached child: its own session (setsid),
// stdout/stderr redirected to logPath (created if missing, appended
// otherwise), and stdin closed. The parent MUST NOT Wait — the child
// outlives it. Returns the child pid so the caller can write it to the
// pidfile.
//
// Linux-only (SysProcAttr.Setsid). Callers on other platforms should gate
// the invocation or accept a non-detached foreground run.
func Daemonize(argv []string, logPath string) (int, error) {
	if len(argv) == 0 {
		return 0, errors.New("watch: Daemonize requires a non-empty argv")
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return 0, fmt.Errorf("watch: prepare log dir: %w", err)
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, fmt.Errorf("watch: open log file %s: %w", logPath, err)
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdout = f
	cmd.Stderr = f
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		_ = f.Close()
		return 0, fmt.Errorf("watch: spawn detached child: %w", err)
	}
	// Release our copy of the log fd — the child inherited its own.
	_ = f.Close()

	// Snapshot pid BEFORE Release — Release zeroes Process.Pid.
	pid := cmd.Process.Pid

	// Release the child so it doesn't become a zombie when our parent exits.
	// Non-fatal on error: child is already running; worst case the runtime
	// keeps a process record around briefly.
	_ = cmd.Process.Release()
	return pid, nil
}
