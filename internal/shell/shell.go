package shell

import (
	"os"
	"strings"

	"github.com/phlisg/frank/internal/config"
)

const (
	dc       = "docker compose --project-directory . -f .frank/compose.yaml"
	execSail = dc + " exec --user sail laravel.test"
)

// aliasTable defines all aliases managed by frank activate/deactivate.
// service is empty for core aliases; non-empty entries are only activated
// when cfg.HasService(service) is true.
var aliasTable = []struct {
	name    string
	cmd     string
	service string
}{
	// Core aliases — always present regardless of service selection
	{"artisan", execSail + " php artisan", ""},
	{"composer", execSail + " composer", ""},
	{"php", execSail + " php", ""},
	{"tinker", execSail + " php artisan tinker", ""},
	{"npm", execSail + " npm", ""},
	{"pnpm", execSail + " pnpm", ""},
	{"bun", execSail + " bun", ""},
	// Service-conditional aliases
	{"psql", dc + " exec pgsql psql -U sail", "pgsql"},
	{"mysql", dc + " exec db mysql -u root -proot", "mysql"},
	{"mariadb", dc + " exec mariadb mariadb -u root -proot", "mariadb"},
	{"redis-cli", dc + " exec redis redis-cli", "redis"},
}

// Activate returns eval-able shell aliases for the current project.
// Core aliases always present; service aliases added based on cfg.
func Activate(cfg *config.Config) string {
	var sb strings.Builder
	for _, a := range aliasTable {
		if a.service != "" && !cfg.HasService(a.service) {
			continue
		}
		alias(&sb, a.name, a.cmd)
	}
	return sb.String()
}

// Deactivate returns unalias commands for all aliases frank can ever set.
func Deactivate() string {
	var sb strings.Builder
	for _, a := range aliasTable {
		sb.WriteString("unalias ")
		sb.WriteString(a.name)
		sb.WriteString(" 2>/dev/null || true\n")
	}
	return sb.String()
}

// ShellSetup returns eval-able shell hooks for auto-activating on directory change.
// If shell is empty, it is detected from $SHELL.
func ShellSetup(shell string) string {
	if shell == "" {
		shell = detectShell()
	}
	switch shell {
	case "zsh":
		return zshHook()
	default:
		return bashHook()
	}
}

func alias(builder *strings.Builder, name, cmd string) {
	builder.WriteString("alias ")
	builder.WriteString(name)
	builder.WriteString("='")
	builder.WriteString(cmd)
	builder.WriteString("'\n")
}

func detectShell() string {
	if shellPath := os.Getenv("SHELL"); strings.Contains(shellPath, "zsh") {
		return "zsh"
	}
	return "bash"
}

func zshHook() string {
	return `_frank_setup() {
  chpwd_functions+=(frank_chpwd)
  eval "$(frank completion zsh)"
  frank_chpwd
}
frank_chpwd() {
  if [[ -f frank.yaml ]]; then
    eval "$(frank activate)"
  else
    eval "$(frank deactivate)"
  fi
}
_frank_precmd_init() {
  if command -v frank &>/dev/null; then
    precmd_functions=("${precmd_functions[@]:#_frank_precmd_init}")
    _frank_setup
  fi
}
if command -v frank &>/dev/null; then
  _frank_setup
else
  precmd_functions+=(_frank_precmd_init)
fi
`
}

func bashHook() string {
	return `_frank_setup() {
  if [[ -n "$PROMPT_COMMAND" ]]; then
    PROMPT_COMMAND="frank_chpwd;${PROMPT_COMMAND}"
  else
    PROMPT_COMMAND="frank_chpwd"
  fi
  eval "$(frank completion bash)"
  frank_chpwd
}
frank_chpwd() {
  if [[ -f frank.yaml ]]; then
    eval "$(frank activate)"
  else
    eval "$(frank deactivate)"
  fi
}
_frank_prompt_init() {
  if command -v frank &>/dev/null; then
    PROMPT_COMMAND="${PROMPT_COMMAND//_frank_prompt_init;/}"
    _frank_setup
  fi
}
if command -v frank &>/dev/null; then
  _frank_setup
else
  if [[ -n "$PROMPT_COMMAND" ]]; then
    PROMPT_COMMAND="_frank_prompt_init;${PROMPT_COMMAND}"
  else
    PROMPT_COMMAND="_frank_prompt_init"
  fi
fi
`
}
