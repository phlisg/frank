package shell

import (
	"os"
	"strings"

	"github.com/phlisg/frank-cli/internal/config"
)

// Activate returns eval-able shell aliases for the current project.
// Core aliases always present; service aliases added based on cfg.
func Activate(cfg *config.Config) string {
	var b strings.Builder

	// Core aliases — always present regardless of service selection
	alias(&b, "artisan", "docker compose exec laravel.test php artisan")
	alias(&b, "composer", "docker compose exec laravel.test composer")
	alias(&b, "php", "docker compose exec laravel.test php")
	alias(&b, "tinker", "docker compose exec laravel.test php artisan tinker")
	alias(&b, "npm", "docker compose exec laravel.test npm")
	alias(&b, "bun", "docker compose exec laravel.test bun")

	// Service-conditional aliases
	if cfg.HasService("pgsql") {
		alias(&b, "psql", "docker compose exec pgsql psql -U sail")
	}
	if cfg.HasService("mysql") {
		alias(&b, "mysql", "docker compose exec db mysql -u root -proot")
	}
	if cfg.HasService("mariadb") {
		alias(&b, "mariadb", "docker compose exec mariadb mariadb -u root -proot")
	}
	if cfg.HasService("redis") {
		alias(&b, "redis-cli", "docker compose exec redis redis-cli")
	}

	return b.String()
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

func alias(b *strings.Builder, name, cmd string) {
	b.WriteString("alias ")
	b.WriteString(name)
	b.WriteString("='")
	b.WriteString(cmd)
	b.WriteString("'\n")
}

func detectShell() string {
	if s := os.Getenv("SHELL"); strings.Contains(s, "zsh") {
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
