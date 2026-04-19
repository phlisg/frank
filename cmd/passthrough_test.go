package cmd

import "testing"

func TestSplitPassthrough_NoSeparator(t *testing.T) {
	got := splitPassthrough(nil, []string{"a", "b"})
	if got != nil {
		t.Errorf("no '--' should return nil, got %v", got)
	}
}

func TestSplitPassthrough_TokenPresent(t *testing.T) {
	got := splitPassthrough(nil, []string{"a", "--", "b", "c"})
	want := []string{"b", "c"}
	if !equalSlice(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSplitPassthrough_LeadingSeparator(t *testing.T) {
	got := splitPassthrough(nil, []string{"--", "--force-recreate"})
	want := []string{"--force-recreate"}
	if !equalSlice(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSplitPassthrough_TrailingSeparator(t *testing.T) {
	got := splitPassthrough(nil, []string{"a", "b", "--"})
	if len(got) != 0 {
		t.Errorf("trailing '--' should yield empty tail, got %v", got)
	}
}

func TestStripDirFlag_NoDir(t *testing.T) {
	_, rest := stripDirFlag([]string{"up", "-d"})
	want := []string{"up", "-d"}
	if !equalSlice(rest, want) {
		t.Errorf("rest = %v, want %v", rest, want)
	}
}

func TestStripDirFlag_WithDir(t *testing.T) {
	dir, rest := stripDirFlag([]string{"--dir", "/tmp/foo", "build", "--no-cache"})
	if dir != "/tmp/foo" {
		t.Errorf("dir = %q, want /tmp/foo", dir)
	}
	want := []string{"build", "--no-cache"}
	if !equalSlice(rest, want) {
		t.Errorf("rest = %v, want %v", rest, want)
	}
}

func TestStripDirFlag_DirInMiddle(t *testing.T) {
	dir, rest := stripDirFlag([]string{"build", "--dir", "/x", "--no-cache"})
	if dir != "/x" {
		t.Errorf("dir = %q, want /x", dir)
	}
	want := []string{"build", "--no-cache"}
	if !equalSlice(rest, want) {
		t.Errorf("rest = %v, want %v", rest, want)
	}
}

func TestStripDirFlag_DirWithoutValue(t *testing.T) {
	// Trailing --dir with no value: leave it in rest rather than panic.
	_, rest := stripDirFlag([]string{"build", "--dir"})
	want := []string{"build", "--dir"}
	if !equalSlice(rest, want) {
		t.Errorf("rest = %v, want %v", rest, want)
	}
}
