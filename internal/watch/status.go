package watch

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// PidfileName is the basename of the watcher pidfile. Lives under .frank/
// inside the project root. Exported so other packages (cmd/) can build
// paths without duplicating the string.
const PidfileName = "watch.pid"

// PidfilePath returns the absolute pidfile path for the given project root.
func PidfilePath(projectRoot string) string {
	return filepath.Join(projectRoot, ".frank", PidfileName)
}

// WritePidfile writes pid to path, creating the parent dir if necessary.
func WritePidfile(path string, pid int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0o644)
}

// ReadPidfile returns the pid stored in path. Returns (0, nil) when the
// pidfile does not exist (the "stopped" state). Malformed content surfaces
// as an error so the caller can decide whether to treat it as stale.
func ReadPidfile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	s := strings.TrimSpace(string(data))
	if s == "" {
		return 0, nil
	}
	pid, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("pidfile %q: invalid pid %q", path, s)
	}
	return pid, nil
}

// pidAlive reports whether the given pid is running. Uses Signal(0) — a
// no-op probe that returns ESRCH for missing pids and nil for live ones.
// EPERM is treated as alive: the process exists, we just lack permission
// to signal it (e.g. pid 1 from a non-root watcher). Without this the
// already-running guard would happily overwrite a valid pidfile whose
// owner happened to belong to a different uid.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	return errors.Is(err, syscall.EPERM)
}

// StatusState captures the cross-checked state of the host watcher.
// Derived from the pidfile + laravel.test compose container liveness.
type StatusState int

const (
	// StatusStopped means no live watcher (pidfile missing, empty, or
	// references a dead pid). Callers clean any stale pidfile on this path.
	StatusStopped StatusState = iota

	// StatusRunning means the pidfile references a live process AND the
	// laravel.test container is up — the normal healthy state.
	StatusRunning

	// StatusOrphaned means the pidfile references a live process but the
	// laravel.test container is gone. The checker SIGTERMs the process and
	// unlinks the pidfile so the next `frank up` starts cleanly.
	StatusOrphaned
)

// String renders StatusState for --status output.
func (s StatusState) String() string {
	switch s {
	case StatusStopped:
		return "stopped"
	case StatusRunning:
		return "running"
	case StatusOrphaned:
		return "orphaned"
	default:
		return "unknown"
	}
}

// Status is the checker's result.
type Status struct {
	State     StatusState
	PID       int
	StartedAt time.Time // pidfile mtime; zero when stopped
}

// Uptime returns time since the pidfile was written. Zero when StartedAt
// is zero (stopped state).
func (s Status) Uptime() time.Duration {
	if s.StartedAt.IsZero() {
		return 0
	}
	return time.Since(s.StartedAt)
}

// StatusChecker cross-checks the watcher pidfile against the laravel.test
// container to classify the watcher's state. Side-effects are load-bearing:
// orphaned → SIGTERM + unlink pidfile; stopped with stale pidfile → unlink.
//
// Seams (pidAliveFn, killFn, removeFn) are exported-through-constructor for
// the prod path and overridable by tests in-package.
type StatusChecker struct {
	PidfilePath    string
	LaravelRunning func() bool

	pidAliveFn func(int) bool
	killFn     func(int) error
	removeFn   func(string) error
	statFn     func(string) (os.FileInfo, error)
}

// NewStatusChecker constructs a checker for the given project root. The
// laravelRunning callback is invoked at Check time and should answer "is
// the laravel.test container up right now?" — typically via
// `docker compose ps -q laravel.test`.
func NewStatusChecker(projectRoot string, laravelRunning func() bool) *StatusChecker {
	return &StatusChecker{
		PidfilePath:    PidfilePath(projectRoot),
		LaravelRunning: laravelRunning,
		pidAliveFn:     pidAlive,
		killFn:         func(pid int) error { return syscall.Kill(pid, syscall.SIGTERM) },
		removeFn:       os.Remove,
		statFn:         os.Stat,
	}
}

// Check reads the pidfile, cross-checks liveness, and returns the resolved
// status. See StatusState docs for the mapping + side effects.
func (c *StatusChecker) Check() (Status, error) {
	pid, err := ReadPidfile(c.PidfilePath)
	if err != nil {
		// Malformed pidfile counts as stopped; try to unlink so the next
		// run starts clean. Return the parse error so the caller can warn.
		_ = c.removeFn(c.PidfilePath)
		return Status{State: StatusStopped}, err
	}

	if pid == 0 {
		return Status{State: StatusStopped}, nil
	}

	if !c.pidAliveFn(pid) {
		// Stale pidfile — process died without cleanup.
		_ = c.removeFn(c.PidfilePath)
		return Status{State: StatusStopped}, nil
	}

	// Pid is alive. Cross-check the laravel.test container.
	startedAt := c.pidfileMTime()

	if c.LaravelRunning == nil || !c.LaravelRunning() {
		// Orphan: watcher survived the container it was meant to reload.
		// SIGTERM the process; unlink the pidfile. Don't fail hard if the
		// signal errors (already exiting, race with shutdown).
		_ = c.killFn(pid)
		_ = c.removeFn(c.PidfilePath)
		return Status{State: StatusOrphaned, PID: pid, StartedAt: startedAt}, nil
	}

	return Status{State: StatusRunning, PID: pid, StartedAt: startedAt}, nil
}

func (c *StatusChecker) pidfileMTime() time.Time {
	info, err := c.statFn(c.PidfilePath)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}
