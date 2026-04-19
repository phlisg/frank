package shell

import (
	"strings"
	"testing"

	"github.com/phlisg/frank/internal/config"
)

func TestActivate(t *testing.T) {
	cfg := &config.Config{Services: []string{"pgsql", "mailpit"}}
	out := Activate(cfg)
	for _, want := range []string{"alias npm=", "alias pnpm=", "alias bun=", "alias artisan=", "alias composer=", "alias psql="} {
		if !strings.Contains(out, want) {
			t.Errorf("Activate output missing %q; got:\n%s", want, out)
		}
	}
	for _, bad := range []string{"alias mysql=", "alias mariadb=", "alias redis-cli="} {
		if strings.Contains(out, bad) {
			t.Errorf("Activate output contains %q but service not configured", bad)
		}
	}
}

func TestShellSetup_Zsh(t *testing.T) {
	output := ShellSetup("zsh")

	checks := []struct {
		pattern string
		desc    string
	}{
		{"_frank_setup", "must define _frank_setup function"},
		{"frank_chpwd", "must define frank_chpwd function"},
		{"_frank_precmd_init", "must define _frank_precmd_init function"},
		{"command -v frank", "must check for frank in PATH using command -v"},
		{"chpwd_functions", "must register frank_chpwd into chpwd_functions"},
		{"precmd_functions", "must register _frank_precmd_init into precmd_functions"},
		{"frank completion zsh", "must call frank completion zsh"},
	}

	for _, c := range checks {
		if !strings.Contains(output, c.pattern) {
			t.Errorf("zsh hook: %s — expected output to contain %q, got:\n%s", c.desc, c.pattern, output)
		}
	}

	// The completion call must appear inside _frank_setup, not at top-level eval time.
	// We verify this by checking that the completion line is NOT the first occurrence
	// of "eval" in the output (i.e., it is guarded inside a function body).
	// More precisely: the string `eval "$(frank completion zsh)"` must only appear
	// after the opening of `_frank_setup`, never before it.
	setupIdx := strings.Index(output, "_frank_setup")
	completionIdx := strings.Index(output, `frank completion zsh`)
	if completionIdx != -1 && setupIdx != -1 && completionIdx < setupIdx {
		t.Errorf("zsh hook: completion call must be inside _frank_setup, not before it (top-level eval is the old bug)")
	}
	if setupIdx == -1 {
		t.Errorf("zsh hook: _frank_setup not found, cannot verify completion placement")
	}
}

func TestShellSetup_Bash(t *testing.T) {
	output := ShellSetup("bash")

	checks := []struct {
		pattern string
		desc    string
	}{
		{"_frank_setup", "must define _frank_setup function"},
		{"frank_chpwd", "must define frank_chpwd function"},
		{"_frank_prompt_init", "must define _frank_prompt_init function"},
		{"command -v frank", "must check for frank in PATH using command -v"},
		{"PROMPT_COMMAND", "must use PROMPT_COMMAND for registration (not precmd_functions)"},
		{"frank completion bash", "must call frank completion bash"},
		{"frank_chpwd", "must call frank_chpwd for initial run"},
	}

	for _, c := range checks {
		if !strings.Contains(output, c.pattern) {
			t.Errorf("bash hook: %s — expected output to contain %q, got:\n%s", c.desc, c.pattern, output)
		}
	}

	// Bash hook must NOT use zsh-specific precmd_functions
	if strings.Contains(output, "precmd_functions") {
		t.Errorf("bash hook: must not use precmd_functions (zsh-only); use PROMPT_COMMAND instead, got:\n%s", output)
	}
}

func TestShellSetup_DefaultDetect(t *testing.T) {
	t.Run("detects zsh from SHELL env", func(t *testing.T) {
		t.Setenv("SHELL", "/bin/zsh")
		output := ShellSetup("")

		if !strings.Contains(output, "_frank_precmd_init") {
			t.Errorf("default detect with SHELL=/bin/zsh: expected zsh hook (containing _frank_precmd_init), got:\n%s", output)
		}
		if strings.Contains(output, "PROMPT_COMMAND") {
			t.Errorf("default detect with SHELL=/bin/zsh: expected zsh hook, but got bash-specific PROMPT_COMMAND, got:\n%s", output)
		}
	})

	t.Run("detects bash from SHELL env", func(t *testing.T) {
		t.Setenv("SHELL", "/bin/bash")
		output := ShellSetup("")

		if !strings.Contains(output, "PROMPT_COMMAND") {
			t.Errorf("default detect with SHELL=/bin/bash: expected bash hook (containing PROMPT_COMMAND), got:\n%s", output)
		}
		if strings.Contains(output, "_frank_precmd_init") {
			t.Errorf("default detect with SHELL=/bin/bash: expected bash hook, but got zsh-specific _frank_precmd_init, got:\n%s", output)
		}
	})
}
