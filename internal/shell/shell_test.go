package shell

import (
	"strings"
	"testing"

	"github.com/phlisg/frank/internal/config"
)

func testConfig(services ...string) *config.Config {
	return &config.Config{Services: services}
}

func TestActivate(t *testing.T) {
	t.Run("core aliases present", func(t *testing.T) {
		cfg := testConfig("pgsql", "mailpit")
		output := Activate(cfg)

		for _, name := range []string{"artisan", "composer", "php", "tinker", "npm", "bun"} {
			if !strings.Contains(output, "alias "+name+"=") {
				t.Errorf("expected core alias %q in output:\n%s", name, output)
			}
		}
	})

	t.Run("service-conditional aliases", func(t *testing.T) {
		cfg := testConfig("pgsql")
		output := Activate(cfg)

		if !strings.Contains(output, "alias psql=") {
			t.Errorf("expected psql alias when pgsql service present:\n%s", output)
		}
		if strings.Contains(output, "alias mysql=") {
			t.Errorf("did not expect mysql alias without mysql service:\n%s", output)
		}
	})

	t.Run("custom container alias", func(t *testing.T) {
		cfg := testConfig("pgsql")
		cfg.Aliases = map[string]config.Alias{
			"pest": {Cmd: "vendor/bin/pest", Host: false},
		}
		output := Activate(cfg)

		if !strings.Contains(output, "alias pest='"+execSail+" vendor/bin/pest'") {
			t.Errorf("expected container custom alias for pest:\n%s", output)
		}
	})

	t.Run("custom host alias", func(t *testing.T) {
		cfg := testConfig("pgsql")
		cfg.Aliases = map[string]config.Alias{
			"deploy": {Cmd: "bash deploy.sh", Host: true},
		}
		output := Activate(cfg)

		if !strings.Contains(output, "alias deploy='bash deploy.sh'") {
			t.Errorf("expected host custom alias for deploy:\n%s", output)
		}
	})

	t.Run("custom aliases sorted", func(t *testing.T) {
		cfg := testConfig()
		cfg.Aliases = map[string]config.Alias{
			"zz":   {Cmd: "echo zz", Host: true},
			"aa":   {Cmd: "echo aa", Host: true},
			"mm":   {Cmd: "echo mm", Host: true},
		}
		output := Activate(cfg)
		aaIdx := strings.Index(output, "alias aa=")
		mmIdx := strings.Index(output, "alias mm=")
		zzIdx := strings.Index(output, "alias zz=")
		if aaIdx == -1 || mmIdx == -1 || zzIdx == -1 {
			t.Fatalf("missing custom aliases in output:\n%s", output)
		}
		if !(aaIdx < mmIdx && mmIdx < zzIdx) {
			t.Errorf("custom aliases not in sorted order:\n%s", output)
		}
	})
}

func TestActivate_FRANK_ALIASES(t *testing.T) {
	cfg := testConfig("pgsql")
	cfg.Aliases = map[string]config.Alias{
		"pest": {Cmd: "vendor/bin/pest", Host: false},
	}
	output := Activate(cfg)

	// Must contain _FRANK_ALIASES line
	if !strings.Contains(output, `_FRANK_ALIASES="`) {
		t.Fatalf("expected _FRANK_ALIASES variable in output:\n%s", output)
	}

	// Extract the value
	start := strings.Index(output, `_FRANK_ALIASES="`) + len(`_FRANK_ALIASES="`)
	end := strings.Index(output[start:], `"`)
	aliasLine := output[start : start+end]

	for _, name := range []string{"artisan", "composer", "php", "tinker", "npm", "bun", "psql", "pest"} {
		if !strings.Contains(aliasLine, name) {
			t.Errorf("_FRANK_ALIASES missing %q, got: %s", name, aliasLine)
		}
	}

	// _FRANK_ALIASES must appear before alias lines
	aliasVarIdx := strings.Index(output, `_FRANK_ALIASES="`)
	firstAlias := strings.Index(output, "alias ")
	if aliasVarIdx >= firstAlias {
		t.Errorf("_FRANK_ALIASES must appear before alias definitions")
	}
}

func TestActivate_SingleQuoteEscape(t *testing.T) {
	cfg := testConfig()
	cfg.Aliases = map[string]config.Alias{
		"tricky": {Cmd: "echo 'hello'", Host: true},
	}
	output := Activate(cfg)

	// Single quotes in cmd must be escaped as '\''
	if !strings.Contains(output, `'\''`) {
		t.Errorf("expected single quote escaping ('\\''') in output:\n%s", output)
	}
	if !strings.Contains(output, "alias tricky=") {
		t.Errorf("expected tricky alias in output:\n%s", output)
	}
}

func TestDeactivate(t *testing.T) {
	output := Deactivate()

	if !strings.Contains(output, "for _a in $_FRANK_ALIASES") {
		t.Errorf("expected _FRANK_ALIASES loop in deactivate output:\n%s", output)
	}
	if !strings.Contains(output, "unset _FRANK_ALIASES") {
		t.Errorf("expected unset _FRANK_ALIASES in deactivate output:\n%s", output)
	}
	// Must NOT contain hardcoded unalias lines
	if strings.Contains(output, "unalias artisan") {
		t.Errorf("deactivate must not hardcode alias names:\n%s", output)
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
		{"frank config shell completion zsh", "must call frank config shell completion zsh"},
		{"frank config shell activate", "must call frank config shell activate"},
		{"frank config shell deactivate", "must call frank config shell deactivate"},
	}

	for _, c := range checks {
		if !strings.Contains(output, c.pattern) {
			t.Errorf("zsh hook: %s — expected output to contain %q, got:\n%s", c.desc, c.pattern, output)
		}
	}

	// The completion call must appear inside _frank_setup, not at top-level eval time.
	setupIdx := strings.Index(output, "_frank_setup")
	completionIdx := strings.Index(output, `frank config shell completion zsh`)
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
		{"frank config shell completion bash", "must call frank config shell completion bash"},
		{"frank config shell activate", "must call frank config shell activate"},
		{"frank config shell deactivate", "must call frank config shell deactivate"},
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
