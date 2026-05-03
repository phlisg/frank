package cmd

import (
	"strings"
	"testing"
)

func TestAdhocQueueName(t *testing.T) {
	name := adhocQueueName(1700000000, 3)
	want := "queue.adhoc.1700000000.3"
	if name != want {
		t.Errorf("adhocQueueName = %q, want %q", name, want)
	}
}

func TestAdhocScheduleName(t *testing.T) {
	name := adhocScheduleName(1700000000)
	want := "schedule.adhoc.1700000000"
	if name != want {
		t.Errorf("adhocScheduleName = %q, want %q", name, want)
	}
}

func TestParseWorkerList_PartitionsByLabel(t *testing.T) {
	in := strings.Join([]string{
		"myapp-queue.default.1-1\tdeclared\tqueue.default.1",
		"myapp-queue.default.2-1\tdeclared\tqueue.default.2",
		"myapp-schedule-1\tdeclared\tschedule",
		"queue.adhoc.1700000000.1\tadhoc\t",
		"schedule.adhoc.1700000000\tadhoc\t",
	}, "\n")

	declared, adhoc := parseWorkerList(in)

	wantDeclared := []string{"queue.default.1", "queue.default.2", "schedule"}
	wantAdhoc := []string{"queue.adhoc.1700000000.1", "schedule.adhoc.1700000000"}

	if !equalSlice(declared, wantDeclared) {
		t.Errorf("declared = %v, want %v", declared, wantDeclared)
	}
	if !equalSlice(adhoc, wantAdhoc) {
		t.Errorf("adhoc = %v, want %v", adhoc, wantAdhoc)
	}
}

func TestParseWorkerList_EmptyAndBlankLines(t *testing.T) {
	declared, adhoc := parseWorkerList("")
	if declared != nil || adhoc != nil {
		t.Errorf("empty input should yield nil slices, got declared=%v adhoc=%v", declared, adhoc)
	}

	declared, adhoc = parseWorkerList("\n\n  \n")
	if declared != nil || adhoc != nil {
		t.Errorf("blank input should yield nil slices, got declared=%v adhoc=%v", declared, adhoc)
	}
}

func TestParseWorkerList_MissingKindDefaultsDeclared(t *testing.T) {
	in := "legacy-worker-1\t\tlegacy.worker"
	declared, adhoc := parseWorkerList(in)
	if len(adhoc) != 0 {
		t.Errorf("empty kind should not classify as adhoc: %v", adhoc)
	}
	if len(declared) != 1 || declared[0] != "legacy.worker" {
		t.Errorf("declared = %v, want [legacy.worker]", declared)
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
