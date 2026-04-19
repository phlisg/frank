package workertop

import (
	"context"
	"errors"
	"io"
	"reflect"
	"sync"
	"testing"
	"time"
)

// fakeExec captures the argv it was called with and returns a
// caller-controlled stdout pipe + wait function. It mimics the real
// exec-backed CmdStartFn without touching docker.
type fakeExec struct {
	mu         sync.Mutex
	gotName    string
	gotArgs    []string
	stdout     io.ReadCloser
	startErr   error
	waitSignal chan struct{} // closed when wait() should return
	waitErr    error
}

func (f *fakeExec) fn(ctx context.Context, name string, args ...string) (io.ReadCloser, func() error, error) {
	f.mu.Lock()
	f.gotName = name
	f.gotArgs = append([]string(nil), args...)
	f.mu.Unlock()
	if f.startErr != nil {
		return nil, nil, f.startErr
	}
	wait := func() error {
		if f.waitSignal != nil {
			<-f.waitSignal
		}
		return f.waitErr
	}
	// When ctx is cancelled, release wait so Run can return promptly —
	// this mirrors exec.CommandContext killing the real process.
	if f.waitSignal != nil {
		go func() {
			<-ctx.Done()
			select {
			case <-f.waitSignal:
			default:
				close(f.waitSignal)
			}
		}()
	}
	return f.stdout, wait, nil
}

// argvFor runs NewLogsReader and returns the full argv it would invoke.
func argvFor(t *testing.T, spec PaneSpec) []string {
	t.Helper()
	var captured []string
	f := &fakeExec{
		stdout: io.NopCloser(emptyReader{}),
	}
	// Start reader with a cancelled ctx so it bails immediately.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := NewLogsReader(spec, func(ctx context.Context, name string, args ...string) (io.ReadCloser, func() error, error) {
		captured = append([]string{name}, args...)
		return f.fn(ctx, name, args...)
	})
	r.Run(ctx)
	return captured
}

// emptyReader returns EOF on first Read — simulates a subprocess that
// produced no output.
type emptyReader struct{}

func (emptyReader) Read(p []byte) (int, error) { return 0, io.EOF }

func TestLogsReader_Declared(t *testing.T) {
	spec := PaneSpec{Kind: KindQueue, Name: "laravel.queue.default.1"}

	// argv assertion
	argv := argvFor(t, spec)
	want := []string{
		"docker", "compose", "--project-directory", ".", "-f", ".frank/compose.yaml",
		"logs", "-f", "--no-log-prefix", "laravel.queue.default.1",
	}
	if !reflect.DeepEqual(argv, want) {
		t.Fatalf("declared argv mismatch:\n got: %q\nwant: %q", argv, want)
	}

	// streaming behaviour: pipe in three lines, expect three LogLines.
	pr, pw := io.Pipe()
	f := &fakeExec{stdout: pr, waitSignal: make(chan struct{})}
	r := NewLogsReader(spec, f.fn)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go r.Run(ctx)

	// Writer goroutine: emit three lines, then close the pipe to
	// signal EOF and let wait() complete.
	go func() {
		_, _ = pw.Write([]byte("first\nsecond\nthird\n"))
		_ = pw.Close()
		// Stdout EOF alone isn't wait() returning — release it too.
		close(f.waitSignal)
	}()

	var got []LogLine
	timeout := time.After(2 * time.Second)
loop:
	for {
		select {
		case line, ok := <-r.Lines():
			if !ok {
				break loop
			}
			got = append(got, line)
		case <-timeout:
			t.Fatalf("timed out waiting for log lines; got %d so far: %+v", len(got), got)
		}
	}

	wantLines := []LogLine{
		{PaneID: "laravel.queue.default.1", Line: "first"},
		{PaneID: "laravel.queue.default.1", Line: "second"},
		{PaneID: "laravel.queue.default.1", Line: "third"},
	}
	if !reflect.DeepEqual(got, wantLines) {
		t.Fatalf("log lines mismatch:\n got: %+v\nwant: %+v", got, wantLines)
	}

	select {
	case <-r.Done():
	case <-time.After(time.Second):
		t.Fatal("Done() did not close after EOF")
	}
}

func TestLogsReader_Adhoc(t *testing.T) {
	spec := PaneSpec{Kind: KindAdhoc, Name: "laravel.queue.default.1.adhoc"}

	argv := argvFor(t, spec)
	want := []string{"docker", "logs", "-f", "laravel.queue.default.1.adhoc"}
	if !reflect.DeepEqual(argv, want) {
		t.Fatalf("adhoc argv mismatch:\n got: %q\nwant: %q", argv, want)
	}

	pr, pw := io.Pipe()
	f := &fakeExec{stdout: pr, waitSignal: make(chan struct{})}
	r := NewLogsReader(spec, f.fn)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go r.Run(ctx)

	go func() {
		_, _ = pw.Write([]byte("a\nb\nc\n"))
		_ = pw.Close()
		close(f.waitSignal)
	}()

	var got []LogLine
	timeout := time.After(2 * time.Second)
loop:
	for {
		select {
		case line, ok := <-r.Lines():
			if !ok {
				break loop
			}
			got = append(got, line)
		case <-timeout:
			t.Fatalf("timed out; got %d: %+v", len(got), got)
		}
	}

	wantLines := []LogLine{
		{PaneID: "laravel.queue.default.1.adhoc", Line: "a"},
		{PaneID: "laravel.queue.default.1.adhoc", Line: "b"},
		{PaneID: "laravel.queue.default.1.adhoc", Line: "c"},
	}
	if !reflect.DeepEqual(got, wantLines) {
		t.Fatalf("log lines mismatch:\n got: %+v\nwant: %+v", got, wantLines)
	}

	select {
	case <-r.Done():
	case <-time.After(time.Second):
		t.Fatal("Done() did not close after EOF")
	}
}

func TestLogsReader_Cancel(t *testing.T) {
	spec := PaneSpec{Kind: KindQueue, Name: "laravel.queue.default.1"}

	pr, pw := io.Pipe()
	f := &fakeExec{stdout: pr, waitSignal: make(chan struct{})}
	r := NewLogsReader(spec, f.fn)

	ctx, cancel := context.WithCancel(context.Background())
	go r.Run(ctx)

	// Feed one line, confirm it arrives, then cancel mid-stream.
	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		// First line: should be delivered.
		if _, err := pw.Write([]byte("first\n")); err != nil {
			return
		}
		// Wait for ctx cancel, then close the pipe so any pending
		// scanner read unblocks even if the select lost the race.
		<-ctx.Done()
		_ = pw.Close()
	}()

	select {
	case line, ok := <-r.Lines():
		if !ok {
			t.Fatal("lines closed before first line delivered")
		}
		if line.Line != "first" {
			t.Fatalf("first line = %q, want %q", line.Line, "first")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first line")
	}

	cancel()

	// Drain: lines channel must close after cancel.
	drainTimeout := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-r.Lines():
			if !ok {
				goto drained
			}
		case <-drainTimeout:
			t.Fatal("lines channel did not close after ctx cancel")
		}
	}
drained:

	select {
	case <-r.Done():
	case <-time.After(time.Second):
		t.Fatal("Done() did not close after ctx cancel")
	}
	<-writeDone
}

func TestLogsReader_StartError(t *testing.T) {
	spec := PaneSpec{Kind: KindQueue, Name: "laravel.queue.default.1"}

	f := &fakeExec{startErr: errors.New("boom")}
	r := NewLogsReader(spec, f.fn)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go r.Run(ctx)

	// Channel must close immediately.
	select {
	case _, ok := <-r.Lines():
		if ok {
			t.Fatal("expected lines channel closed (start error), got a value")
		}
	case <-time.After(time.Second):
		t.Fatal("lines channel did not close after start error")
	}

	select {
	case <-r.Done():
	case <-time.After(time.Second):
		t.Fatal("Done() did not close after start error")
	}
}

func TestLogsReader_EOF(t *testing.T) {
	spec := PaneSpec{Kind: KindQueue, Name: "laravel.queue.default.1"}

	pr, pw := io.Pipe()
	f := &fakeExec{stdout: pr, waitSignal: make(chan struct{})}
	r := NewLogsReader(spec, f.fn)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go r.Run(ctx)

	// Close pipe immediately — process exits with no output. Release
	// wait() so Run can finish.
	_ = pw.Close()
	close(f.waitSignal)

	// Lines channel closes with no values delivered.
	select {
	case _, ok := <-r.Lines():
		if ok {
			t.Fatal("unexpected line on EOF-only stream")
		}
	case <-time.After(time.Second):
		t.Fatal("lines channel did not close after EOF")
	}

	select {
	case <-r.Done():
	case <-time.After(time.Second):
		t.Fatal("Done() did not close after EOF")
	}
}
