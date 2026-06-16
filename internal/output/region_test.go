package output

import "testing"

// TestRegionWrite_LineSplitting checks the io.Writer half of live mode: bytes
// are split on \n, \r stripped, and an unterminated tail held back as partial.
func TestRegionWrite_LineSplitting(t *testing.T) {
	r := &LiveRegion{
		mode:  regionLive,
		lines: make(chan string, 8),
		done:  make(chan struct{}),
	}

	r.Write([]byte("alpha\r\nbeta\n"))
	r.Write([]byte("par"))
	r.Write([]byte("tial\ngamma")) // "gamma" has no newline → stays buffered

	close(r.lines)
	var got []string
	for ln := range r.lines {
		got = append(got, ln)
	}

	want := []string{"alpha", "beta", "partial"}
	if len(got) != len(want) {
		t.Fatalf("got %d lines %q, want %d %q", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
	if string(r.partial) != "gamma" {
		t.Errorf("partial = %q, want %q", r.partial, "gamma")
	}
}
