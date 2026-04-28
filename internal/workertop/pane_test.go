package workertop

import (
	"strings"
	"testing"
)

func TestIsRestartNoise(t *testing.T) {
	noisy := []string{
		"laravel.queue.default.1 exited with code 0 (restarting)",
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
	p := NewPane(PaneSpec{Name: "laravel.queue.default.1", Kind: KindQueue})
	p.viewport.Width = 80
	p.viewport.Height = 20

	p.appendLine("INFO Processing job...")
	p.appendLine("laravel.queue.default.1 exited with code 0 (restarting)")
	p.appendLine("usermod: no changes")
	p.appendLine("INFO Back to work")

	if len(p.buffer) != 3 {
		t.Fatalf("expected 3 buffer lines (log + banner + log), got %d: %v", len(p.buffer), p.buffer)
	}

	if !strings.Contains(p.buffer[1], "RESTART") {
		t.Errorf("expected RESTART banner, got %q", p.buffer[1])
	}
}
