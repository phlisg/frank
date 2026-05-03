package workertop

import (
	"strings"
	"testing"
)

func TestIsRestartNoise(t *testing.T) {
	noisy := []string{
		"queue.default.1 exited with code 0 (restarting)",
		"usermod: no changes",
		"usermod: no change",
	}
	for _, line := range noisy {
		if !isRestartNoise(line) {
			t.Errorf("expected noise match for %q", line)
		}
	}

	clean := []string{
		"2026-04-28 INFO Processing job...",
		"artisan queue:work running",
		"",
	}
	for _, line := range clean {
		if isRestartNoise(line) {
			t.Errorf("false positive noise match for %q", line)
		}
	}
}

func TestAppendLine_CollapsesRestartNoise(t *testing.T) {
	p := NewPane(PaneSpec{Name: "queue.default.1", Kind: KindQueue})
	p.viewport.Width = 80
	p.viewport.Height = 20

	p.appendLine("INFO Processing job...")
	p.appendLine("queue.default.1 exited with code 0 (restarting)")
	p.appendLine("usermod: no changes")
	p.appendLine("INFO Back to work")

	if len(p.buffer) != 3 {
		t.Fatalf("expected 3 buffer lines (log + banner + log), got %d: %v", len(p.buffer), p.buffer)
	}

	if !strings.Contains(p.buffer[1], "RESTART") {
		t.Errorf("expected RESTART banner, got %q", p.buffer[1])
	}
}

func TestSearch_FiltersBufferLines(t *testing.T) {
	p := NewPane(PaneSpec{Name: "queue.default.1", Kind: KindQueue})
	p.viewport.Width = 80
	p.viewport.Height = 20

	p.appendLine("INFO Processing order #123")
	p.appendLine("DEBUG cache hit for key:user:5")
	p.appendLine("INFO Processing order #456")
	p.appendLine("ERROR timeout on order #789")

	p.SetSearch("order")
	content := p.viewport.View()
	if !strings.Contains(content, "order #123") {
		t.Error("search should include matching lines")
	}
	if strings.Contains(content, "cache hit") {
		t.Error("search should exclude non-matching lines")
	}

	p.SetSearch("ERROR")
	content = p.viewport.View()
	if !strings.Contains(content, "timeout") {
		t.Error("ERROR search should match error line")
	}
	if strings.Contains(content, "order #123") {
		t.Error("ERROR search should exclude INFO lines")
	}

	// Case insensitive
	p.SetSearch("error")
	content = p.viewport.View()
	if !strings.Contains(content, "timeout") {
		t.Error("search should be case-insensitive")
	}

	p.ClearSearch()
	content = p.viewport.View()
	if !strings.Contains(content, "cache hit") {
		t.Error("clear should restore all lines")
	}
}

func TestPause_FreezesScrollAndUnpauseCatchesUp(t *testing.T) {
	p := NewPane(PaneSpec{Name: "queue.default.1", Kind: KindQueue})
	p.viewport.Width = 80
	p.viewport.Height = 5

	p.appendLine("line 1")
	p.appendLine("line 2")

	p.TogglePause()
	if !p.paused {
		t.Fatal("expected paused=true after toggle")
	}

	posBefore := p.viewport.YOffset
	for i := 0; i < 20; i++ {
		p.appendLine("scrolling line")
	}
	if p.viewport.YOffset != posBefore {
		t.Errorf("viewport scrolled while paused: was %d, now %d", posBefore, p.viewport.YOffset)
	}

	p.TogglePause()
	if p.paused {
		t.Fatal("expected paused=false after second toggle")
	}
	// After unpause, viewport should be at bottom.
	content := p.viewport.View()
	if !strings.Contains(content, "scrolling line") {
		t.Error("viewport should show recent lines after unpause")
	}
}
