package tool

import (
	"strings"
	"testing"
)

func TestAssembleLefthook_AllTools(t *testing.T) {
	out := AssembleLefthook([]string{"pint", "rector", "larastan", "lefthook"})

	if !strings.Contains(out, "assert_lefthook_installed: true") {
		t.Error("expected assert_lefthook_installed: true")
	}
	if !strings.Contains(out, "post-merge:") {
		t.Error("expected post-merge section")
	}
	if !strings.Contains(out, "pre-commit:") {
		t.Error("expected pre-commit section")
	}
	for _, name := range []string{"pint:", "rector:", "larastan:"} {
		if !strings.Contains(out, name) {
			t.Errorf("expected pre-commit entry for %s", name)
		}
	}
}

func TestAssembleLefthook_SubsetPintOnly(t *testing.T) {
	out := AssembleLefthook([]string{"pint", "lefthook"})

	if !strings.Contains(out, "pre-commit:") {
		t.Error("expected pre-commit section")
	}
	if !strings.Contains(out, "pint:") {
		t.Error("expected pint entry")
	}
	if strings.Contains(out, "rector:") {
		t.Error("unexpected rector entry")
	}
	if strings.Contains(out, "larastan:") {
		t.Error("unexpected larastan entry")
	}
}

func TestAssembleLefthook_NoPhpTools(t *testing.T) {
	out := AssembleLefthook([]string{"lefthook"})

	if !strings.Contains(out, "post-merge:") {
		t.Error("expected post-merge section")
	}
	if strings.Contains(out, "pre-commit:") {
		t.Error("unexpected pre-commit section when no PHP tools selected")
	}
}

func TestAssembleLefthook_PMDetection(t *testing.T) {
	out := AssembleLefthook([]string{"lefthook"})

	// node-install job uses if/elif/else pattern for package manager detection
	if !strings.Contains(out, "if command -v pnpm") {
		t.Error("expected pnpm detection")
	}
	if !strings.Contains(out, "elif command -v bun") {
		t.Error("expected bun detection")
	}
	if !strings.Contains(out, "else npm install") {
		t.Error("expected npm fallback")
	}
}

func TestAssembleLefthook_StageFixed(t *testing.T) {
	out := AssembleLefthook([]string{"pint", "rector", "larastan", "lefthook"})

	// Split into entries to check stage_fixed per tool
	pintIdx := strings.Index(out, "    pint:")
	rectorIdx := strings.Index(out, "    rector:")
	larastanIdx := strings.Index(out, "    larastan:")

	if pintIdx < 0 || rectorIdx < 0 || larastanIdx < 0 {
		t.Fatal("could not locate all three tool entries")
	}

	pintSection := out[pintIdx:rectorIdx]
	rectorSection := out[rectorIdx:larastanIdx]
	larastanSection := out[larastanIdx:]

	if !strings.Contains(pintSection, "stage_fixed: true") {
		t.Error("pint should have stage_fixed: true")
	}
	if !strings.Contains(rectorSection, "stage_fixed: true") {
		t.Error("rector should have stage_fixed: true")
	}
	if strings.Contains(larastanSection, "stage_fixed: true") {
		t.Error("larastan should NOT have stage_fixed: true")
	}
}

func TestLefthookEntry(t *testing.T) {
	tests := []struct {
		name     string
		wantGlob string
		wantRun  string
	}{
		{"pint", `"*.php"`, "frank exec php vendor/bin/pint {staged_files}"},
		{"rector", `"*.php"`, "frank exec php vendor/bin/rector process {staged_files}"},
		{"larastan", `"*.php"`, "frank exec php vendor/bin/phpstan analyse -c phpstan.neon {staged_files}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := lookupTool(tt.name)
			if tool == nil {
				t.Fatalf("tool %q not found in registry", tt.name)
			}
			entry := lefthookEntry(*tool)
			if !strings.Contains(entry, tt.name+":") {
				t.Errorf("entry missing tool name header %q", tt.name+":")
			}
			if !strings.Contains(entry, tt.wantGlob) {
				t.Errorf("entry missing glob %q", tt.wantGlob)
			}
			if !strings.Contains(entry, tt.wantRun) {
				t.Errorf("entry missing run command %q", tt.wantRun)
			}
		})
	}

	// Unknown tool returns empty string
	unknown := Tool{Name: "unknown", Category: "php"}
	if got := lefthookEntry(unknown); got != "" {
		t.Errorf("expected empty string for unknown tool, got %q", got)
	}
}
