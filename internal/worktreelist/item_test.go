package worktreelist

import "testing"

func TestFilterValue(t *testing.T) {
	item := WorktreeItem{Branch: "feature/awesome"}
	if got := item.FilterValue(); got != "feature/awesome" {
		t.Errorf("FilterValue() = %q, want %q", got, "feature/awesome")
	}
}

func TestItemDelegateHeight(t *testing.T) {
	d := ItemDelegate{}
	if got := d.Height(); got != 3 {
		t.Errorf("Height() = %d, want 3", got)
	}
}

func TestItemDelegateSpacing(t *testing.T) {
	d := ItemDelegate{}
	if got := d.Spacing(); got != 1 {
		t.Errorf("Spacing() = %d, want 1", got)
	}
}
