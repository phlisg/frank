package shell

import (
	"os"
	"strings"

	"github.com/phlisg/frank/internal/config"
)

// Activate returns eval-able shell aliases for the current project.
// Core aliases always present; service aliases added based on cfg.
func Activate(cfg *config.Config) string {
	var sb strings.Builder

	// Core aliases — always present regardless of service selection
	alias(&sb, "artisan", "docker compose exec --user sail laravel.test php artisan")
	alias(&sb, "composer", "docker compose exec --user sail laravel.test composer")
	alias(&sb, "php", "docker compose exec --user sail laravel.test php")
	alias(&sb, "tinker", "docker compose exec --user sail laravel.test php artisan tinker")
	alias(&sb, "npm", "docker compose exec --user sail laravel.test npm")
	alias(&sb, "bun", "docker compose exec --user sail laravel.test bun")

	// Service-conditional aliases
	if cfg.HasService("pgsql") {
		alias(&sb, "psql", "docker compose exec pgsql psql -U sail")
	}
	if cfg.HasService("mysql") {
		alias(&sb, "mysql", "docker compose exec db mysql -u root -proot")
	}
	if cfg.HasService("mariadb") {
		alias(&sb, "mariadb", "docker compose exec mariadb mariadb -u root -proot")
	}
	if cfg.HasService("redis") {
		alias(&sb, "redis-cli", "docker compose exec redis redis-cli")
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
	return `frank_chpwd() {
  if [[ -f frank.yaml ]]; then
    eval "$(frank activate)"
  else
    unalias artisan composer php tinker npm bun psql mysql mariadb redis-cli 2>/dev/null || true
  fi
}
chpwd_functions+=(frank_chpwd)
`
}

func bashHook() string {
	return `frank_cd() {
  builtin cd "$@" || return
  if [[ -f frank.yaml ]]; then
    eval "$(frank activate)"
  else
    unalias artisan composer php tinker npm bun psql mysql mariadb redis-cli 2>/dev/null || true
  fi
}
alias cd=frank_cd
`
}
