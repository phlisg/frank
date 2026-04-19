// Package workertop implements the `frank worker top` TUI.
//
// This file owns LogsReader: one goroutine per pane, shelling out to
// either `docker compose logs -f --no-log-prefix <svc>` (for declared
// workers) or `docker logs -f <container>` (for ad-hoc workers), and
// streaming each stdout line over an unbuffered channel. The pane's
// bubbletea model subscribes to that channel and appends lines to its
// viewport.
//
// Two log-source paths is deliberate — see the "Dual log source paths"
// note in docs/superpowers/specs/2026-04-19-worker-top-tui-design.md.
// Unifying on `docker logs -f` would require resolving declared
// workers' container IDs up front, which the discover step already
// does but only at cold start; `docker compose logs` keeps following
// even if a compose service restarts and the underlying ID changes.
package workertop

import (
	"bufio"
	"context"
	"io"
	"os/exec"
)

// composePrefix is the docker-CLI argv prefix that pins every compose
// invocation to the Frank project layout. Duplicated from
// internal/docker so this package doesn't pull in that dependency —
// keeps workertop self-contained.
var composePrefix = []string{"compose", "--project-directory", ".", "-f", ".frank/compose.yaml"}

// LogLine is a single line from a pane's log stream.
type LogLine struct {
	// PaneID equals PaneSpec.Name (container/service name).
	PaneID string
	// Line is the raw log text, ANSI escapes preserved.
	Line string
}

// CmdStartFn starts a subprocess and returns (stdout, wait, err).
//   - stdout: a ReadCloser for the process's standard output.
//   - wait:   blocks until the process exits and returns the exit error.
//   - err:    a start error (non-nil → stdout and wait are unusable).
//
// Tests inject a mock; production wires DefaultCmdStartFn.
type CmdStartFn func(ctx context.Context, name string, args ...string) (io.ReadCloser, func() error, error)

// DefaultCmdStartFn wraps exec.CommandContext. It pipes stdout and
// returns cmd.Wait so callers can reap the process.
func DefaultCmdStartFn(ctx context.Context, name string, args ...string) (io.ReadCloser, func() error, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	return stdout, cmd.Wait, nil
}

// LogsReader streams one pane's log output on a channel. Construct
// via NewLogsReader, then call Run under a cancellable context.
// Consumers read Lines(); Done() signals full shutdown.
type LogsReader struct {
	paneID string
	cmd    []string // docker argv, name first (e.g. cmd[0] == "docker")
	exec   CmdStartFn
	lines  chan LogLine
	done   chan struct{}
}

// NewLogsReader builds a LogsReader. For declared workers
// (KindSchedule, KindQueue) it emits
//
//	docker compose --project-directory . -f .frank/compose.yaml logs -f --no-log-prefix <name>
//
// For ad-hoc workers (KindAdhoc) it emits
//
//	docker logs -f <name>
//
// PaneSpec.Name is the service/container name in both cases.
// If exec is nil, DefaultCmdStartFn is used.
func NewLogsReader(spec PaneSpec, exec CmdStartFn) *LogsReader {
	if exec == nil {
		exec = DefaultCmdStartFn
	}

	var argv []string
	argv = append(argv, "docker")
	if spec.Kind == KindAdhoc {
		argv = append(argv, "logs", "-f", spec.Name)
	} else {
		argv = append(argv, composePrefix...)
		argv = append(argv, "logs", "-f", "--no-log-prefix", spec.Name)
	}

	return &LogsReader{
		paneID: spec.Name,
		cmd:    argv,
		exec:   exec,
		// Unbuffered: slow consumers throttle the reader. bubbletea's
		// message loop drains promptly; a buffered queue would hide
		// backpressure.
		lines: make(chan LogLine),
		done:  make(chan struct{}),
	}
}

// Lines returns the channel of log lines. It is closed when Run exits.
func (r *LogsReader) Lines() <-chan LogLine {
	return r.lines
}

// Done returns a channel that is closed after Run has fully stopped —
// i.e. subprocess reaped, channel closed. Callers wait on this during
// shutdown to confirm no stray goroutines remain.
func (r *LogsReader) Done() <-chan struct{} {
	return r.done
}

// Run drives the log-streaming loop until ctx is cancelled or the
// subprocess exits (EOF). Safe to call exactly once per LogsReader.
//
// Shutdown order on exit:
//  1. Scanner loop ends (EOF, scan error, or ctx cancelled).
//  2. lines channel closes so consumers see EOF.
//  3. wait() reaps the subprocess — this also completes promptly when
//     exec.CommandContext's ctx is cancelled because the runtime sends
//     the process a kill signal.
//  4. done closes.
func (r *LogsReader) Run(ctx context.Context) {
	defer close(r.done)

	stdout, wait, err := r.exec(ctx, r.cmd[0], r.cmd[1:]...)
	if err != nil {
		// Start failed — nothing to read, nothing to wait for. Close
		// lines so consumers detect the failure via a zero-value recv.
		close(r.lines)
		return
	}

	// Read lines in the current goroutine. Closing lines here (under
	// the same goroutine that writes to it) avoids any send-on-closed
	// race with the scanner.
	scanner := bufio.NewScanner(stdout)
	// Default scanner buffer (64 KiB) trips on long JSON log lines;
	// bump to 1 MiB max like the rest of the codebase (see
	// internal/docker/docker.go copyWithPrefix).
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := LogLine{PaneID: r.paneID, Line: scanner.Text()}
		select {
		case <-ctx.Done():
			// Consumer (or whole TUI) is shutting down. Stop reading
			// and fall through to cleanup. Draining stdout here would
			// just delay teardown.
			close(r.lines)
			_ = stdout.Close()
			_ = wait()
			return
		case r.lines <- line:
		}
	}

	// Scanner is done (EOF, scan error, or underlying reader closed).
	close(r.lines)
	_ = stdout.Close()
	_ = wait()
}
