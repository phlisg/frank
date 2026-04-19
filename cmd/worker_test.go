package cmd

import (
	"strings"
	"testing"
)

func TestAdhocQueueName(t *testing.T) {
	name := adhocQueueName(1700000000, 3)
	want := "laravel.queue.adhoc.1700000000.3"
	if name != want {
		t.Errorf("adhocQueueName = %q, want %q", name, want)
	}
}

func TestAdhocScheduleName(t *testing.T) {
	name := adhocScheduleName(1700000000)
	want := "laravel.schedule.adhoc.1700000000"
	if name != want {
		t.Errorf("adhocScheduleName = %q, want %q", name, want)
	}
}

func TestBuildQueueArtisanArgs_Defaults(t *testing.T) {
	got := buildQueueArtisanArgs("default", 0, 0, 0, 0, 0, nil)
	want := []string{"php", "artisan", "queue:work", "--queue=default"}
	if !equalSlice(got, want) {
		t.Errorf("default args = %v, want %v", got, want)
	}
}

func TestBuildQueueArtisanArgs_AllFlags(t *testing.T) {
	got := buildQueueArtisanArgs("high,default", 3, 60, 128, 5, 2, nil)
	joined := strings.Join(got, " ")
	for _, expect := range []string{
		"--queue=high,default",
		"--tries=3",
		"--timeout=60",
		"--memory=128",
		"--sleep=5",
		"--backoff=2",
	} {
		if !strings.Contains(joined, expect) {
			t.Errorf("missing %q in %v", expect, got)
		}
	}
}

func TestBuildQueueArtisanArgs_ZeroFlagsOmitted(t *testing.T) {
	got := buildQueueArtisanArgs("default", 0, 60, 0, 0, 0, nil)
	joined := strings.Join(got, " ")
	if strings.Contains(joined, "--tries=") {
		t.Errorf("--tries should be omitted when zero: %v", got)
	}
	if !strings.Contains(joined, "--timeout=60") {
		t.Errorf("--timeout=60 should be present: %v", got)
	}
}

func TestBuildQueueArtisanArgs_Passthrough(t *testing.T) {
	got := buildQueueArtisanArgs("default", 0, 0, 0, 0, 0, []string{"--once", "--stop-when-empty"})
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "--once") || !strings.Contains(joined, "--stop-when-empty") {
		t.Errorf("passthrough not appended: %v", got)
	}
}

func TestSplitArgs_NoSeparator(t *testing.T) {
	got := splitArgs([]string{"a", "b"})
	if !equalSlice(got, []string{"a", "b"}) {
		t.Errorf("without '--', splitArgs should return input verbatim, got %v", got)
	}
}

func TestSplitArgs_WithSeparator(t *testing.T) {
	got := splitArgs([]string{"a", "--", "b", "c"})
	if !equalSlice(got, []string{"b", "c"}) {
		t.Errorf("after '--', splitArgs should return tail, got %v", got)
	}
}

func equalSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
