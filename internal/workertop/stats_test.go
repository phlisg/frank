package workertop

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParseStatsLine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		want    StatsSample
		wantErr bool
	}{
		{
			name: "valid MiB",
			line: "abc123 42.12% 128MiB / 2GiB",
			want: StatsSample{ContainerID: "abc123", MemPct: 42.12, MemBytes: 128 * 1024 * 1024},
		},
		{
			name: "valid decimal GiB",
			line: "def456 0.00% 1.5GiB / 2GiB",
			want: StatsSample{ContainerID: "def456", MemPct: 0.00, MemBytes: int64(1.5 * float64(1<<30))},
		},
		{
			name: "100 percent saturation",
			line: "ghi789 100.00% 2.0GiB / 2.0GiB",
			want: StatsSample{ContainerID: "ghi789", MemPct: 100.00, MemBytes: int64(2.0 * float64(1<<30))},
		},
		{
			name:    "too few tokens",
			line:    "abc123 42.12% 128MiB",
			wantErr: true,
		},
		{
			name:    "bad percent",
			line:    "abc123 notapct% 128MiB / 2GiB",
			wantErr: true,
		},
		{
			name:    "bad bytes unit",
			line:    "abc123 42.12% 128XB / 2GiB",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseStatsLine(tc.line)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseStatsLine(%q): want error, got nil (sample=%+v)", tc.line, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseStatsLine(%q): unexpected error: %v", tc.line, err)
			}
			if got != tc.want {
				t.Fatalf("parseStatsLine(%q): got %+v, want %+v", tc.line, got, tc.want)
			}
		})
	}
}

func TestParseBytes(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    int64
		wantErr bool
	}{
		{name: "MiB", in: "128MiB", want: 128 * 1024 * 1024},
		{name: "decimal GiB", in: "1.5GiB", want: int64(1.5 * float64(1<<30))},
		{name: "SI GB", in: "2GB", want: 2_000_000_000},
		{name: "KiB", in: "1024KiB", want: 1024 * 1024},
		{name: "bare B", in: "512B", want: 512},
		{name: "unknown unit", in: "128XB", wantErr: true},
		{name: "empty", in: "", wantErr: true},
		{name: "missing number", in: "MiB", wantErr: true},
		{name: "bad number", in: "abcMiB", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseBytes(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseBytes(%q): want error, got %d", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseBytes(%q): unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("parseBytes(%q): got %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestHubRunEmitsSnapshot(t *testing.T) {
	canned := "abc123 42.12% 128MiB / 2GiB\ndef456 10.00% 256MiB / 2GiB\n"

	var (
		mu    sync.Mutex
		calls int
		args  [][]string
	)
	mockExec := func(ctx context.Context, name string, a ...string) ([]byte, error) {
		mu.Lock()
		calls++
		args = append(args, append([]string(nil), a...))
		mu.Unlock()
		return []byte(canned), nil
	}

	hub := NewHub([]string{"abc123", "def456"}, 50*time.Millisecond, mockExec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		hub.Run(ctx)
		close(done)
	}()

	// First snapshot arrives immediately (sampleAndEmit before ticker).
	var snap map[string]StatsSample
	select {
	case snap = <-hub.Updates():
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for initial snapshot")
	}

	if len(snap) != 2 {
		t.Fatalf("snapshot size: got %d, want 2 (snap=%+v)", len(snap), snap)
	}
	if s, ok := snap["abc123"]; !ok || s.MemBytes != 128*1024*1024 || s.MemPct != 42.12 {
		t.Fatalf("abc123 entry wrong: %+v", s)
	}
	if s, ok := snap["def456"]; !ok || s.MemBytes != 256*1024*1024 {
		t.Fatalf("def456 entry wrong: %+v", s)
	}

	// Verify exec args: first must be `stats`, `--no-stream`, `--format`,
	// format string, then container IDs.
	mu.Lock()
	gotArgs := args[0]
	mu.Unlock()
	if len(gotArgs) < 5 {
		t.Fatalf("docker args too short: %v", gotArgs)
	}
	if gotArgs[0] != "stats" || gotArgs[1] != "--no-stream" || gotArgs[2] != "--format" {
		t.Fatalf("docker args prefix wrong: %v", gotArgs)
	}
	if !strings.Contains(gotArgs[3], "{{.ID}}") {
		t.Fatalf("docker format missing ID template: %q", gotArgs[3])
	}
	// container IDs must be present and last.
	tail := gotArgs[len(gotArgs)-2:]
	if tail[0] != "abc123" || tail[1] != "def456" {
		t.Fatalf("container IDs missing from args tail: %v", gotArgs)
	}

	// Subsequent ticks should keep emitting.
	select {
	case <-hub.Updates():
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for second snapshot")
	}

	// Cancel → channel closes quickly and Run returns.
	cancel()
	select {
	case _, ok := <-hub.Updates():
		// Either we got a trailing snapshot or the close. Drain until closed.
		if ok {
			// drain once more for close signal
			select {
			case _, ok2 := <-hub.Updates():
				if ok2 {
					t.Fatal("expected channel to close after cancel")
				}
			case <-time.After(500 * time.Millisecond):
				t.Fatal("timeout waiting for channel close after cancel")
			}
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout: Updates() should close after cancel")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run() did not return after cancel")
	}
}

func TestHubRunNoContainerIDs(t *testing.T) {
	// With no container IDs the hub must NOT call docker; it should
	// block on ctx and return when cancelled.
	called := false
	mockExec := func(ctx context.Context, name string, a ...string) ([]byte, error) {
		called = true
		return nil, fmt.Errorf("should not be called")
	}

	hub := NewHub(nil, 10*time.Millisecond, mockExec)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		hub.Run(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run() did not return after cancel")
	}

	if called {
		t.Fatal("exec was called despite empty container list")
	}

	// Channel must be closed.
	if _, ok := <-hub.Updates(); ok {
		t.Fatal("expected closed Updates channel")
	}
}

func TestHubRunSkipsMalformedLines(t *testing.T) {
	// Mix of valid and garbage lines — hub should keep the valid rows
	// and drop the bad ones without dying.
	canned := "abc123 42.12% 128MiB / 2GiB\ntotally bogus line\ndef456 notapct% 256MiB / 2GiB\nghi789 5.00% 64MiB / 2GiB\n"
	mockExec := func(ctx context.Context, name string, a ...string) ([]byte, error) {
		return []byte(canned), nil
	}

	hub := NewHub([]string{"abc123", "def456", "ghi789"}, 10*time.Millisecond, mockExec)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go hub.Run(ctx)

	select {
	case snap := <-hub.Updates():
		if len(snap) != 2 {
			t.Fatalf("want 2 valid entries, got %d: %+v", len(snap), snap)
		}
		if _, ok := snap["abc123"]; !ok {
			t.Fatal("abc123 missing")
		}
		if _, ok := snap["ghi789"]; !ok {
			t.Fatal("ghi789 missing")
		}
		if _, ok := snap["def456"]; ok {
			t.Fatal("def456 should have been skipped (bad percent)")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for snapshot")
	}
}
