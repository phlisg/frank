# Frank Go CLI Design Spec

> **Date:** 2026-03-25
> **Status:** Approved
> **Goal:** Replace Frank's bash scripts, justfile, and yq dependency with a single Go binary CLI.

## Overview

Frank CLI is a single Go binary that generates and manages Docker-based Laravel development environments. It embeds all templates, parses `frank.yaml` natively, generates standard `compose.yaml` files, and wraps `docker compose` for lifecycle management.

**Host dependencies reduced to:** Docker only. Go is needed only for `go install`.

## Distribution

- `go install github.com/<username>/frank-cli@latest` — installs as `frank`
- Pre-built binaries via GitHub Releases (linux/mac/windows, amd64/arm64)

## Architecture

Flat `internal/` package structure. No hexagonal architecture — clean separation by concern is sufficient for a CLI tool.

```
frank-cli/
├── cmd/                  # Cobra command definitions
│   ├── root.go           # bare `frank` → smart help
│   ├── init.go           # interactive wizard
│   ├── generate.go       # template rendering
│   ├── add.go            # add service
│   ├── remove.go         # remove service
│   ├── install.go        # laravel install
│   ├── up.go             # docker compose up
│   ├── down.go           # docker compose down
│   ├── ps.go             # container status
│   ├── clean.go          # docker compose down -v
│   ├── reset.go          # full project reset
│   ├── activate.go       # output shell aliases
│   ├── shellsetup.go     # output chpwd hook
│   ├── import.go         # sail import
│   └── export.go         # sail export
├── internal/
│   ├── config/           # frank.yaml parsing & defaults
│   ├── template/         # template rendering engine
│   ├── compose/          # compose.yaml generation & merging
│   ├── docker/           # docker compose exec wrapper
│   └── shell/            # activate & shell-setup generation
├── templates/            # embedded via embed.FS
├── main.go
├── go.mod
└── go.sum
```

## Technology Choices

| Aspect | Decision | Rationale |
|---|---|---|
| Language | Go 1.26 | Latest stable, single binary output |
| CLI framework | Cobra | Industry standard (Docker, kubectl, Hugo) |
| Templates | Go `text/template` | Native, no custom syntax needed — replaces `%%..%%` Frankies |
| YAML | Go YAML library | Replaces `yq` host dependency |
| Interactive prompts | huh or survey | For `frank init` wizard only |
| Template embedding | `embed.FS` | Templates ship inside the binary |

## Commands

### Project Setup

| Command | Description |
|---|---|
| `frank init` | Interactive wizard — creates `frank.yaml` with service picker |
| `frank init --sail` | Interactive wizard — creates a Sail-compatible Laravel project with no Frank traces. Prompts for service selection like `artisan sail:install`. |
| `frank generate` | Reads `frank.yaml`, produces `compose.yaml` + Dockerfiles + runtime configs |
| `frank generate --sail` | Same as generate but outputs Sail-compatible naming/structure |
| `frank install` | Runs Laravel installer inside a container (see Install Behavior below) |

### Service Management

| Command | Description |
|---|---|
| `frank add <service>` | Add a service to `frank.yaml` and regenerate |
| `frank remove <service>` | Remove a service from `frank.yaml` and regenerate |

### Lifecycle (Docker Compose wrappers)

| Command | Description |
|---|---|
| `frank up` | `docker compose up -d --build`, then runs `php artisan migrate --force` |
| `frank down` | `docker compose down` |
| `frank ps` | `docker compose ps` — container status |
| `frank clean` | `docker compose down -v` — remove volumes |
| `frank reset` | Full project reset — see Reset Behavior below |

### Shell Integration

| Command | Description |
|---|---|
| `frank activate` | Outputs shell aliases for current project's services |
| `frank shell-setup` | Outputs shell snippet for auto-activation on `cd` |

### Sail Interop

| Command | Description |
|---|---|
| `frank import` | Import from existing Sail docker-compose, then auto-runs `generate` |
| `frank export [path]` | Export current Frank project as Sail-compatible to target path (equivalent to `frank generate --sail` but outputs to a different directory) |

### Global Flags

| Flag | Description |
|---|---|
| `--dir <path>` | Target directory (defaults to current working directory) |

## Config System (`internal/config`)

Parses `frank.yaml` into Go structs with sensible defaults:

```go
type Config struct {
    Version  int                       `yaml:"version"`
    PHP      PHP                       `yaml:"php"`
    Laravel  Laravel                   `yaml:"laravel"`
    Services []string                  `yaml:"services"`
    Config   map[string]ServiceConfig  `yaml:"config"`
}

type PHP struct {
    Version string `yaml:"version"` // default: "8.5"
    Runtime string `yaml:"runtime"` // default: "frankenphp"
}

type Laravel struct {
    Version string `yaml:"version"` // default: "latest"
}

type ServiceConfig struct {
    Port    int    `yaml:"port"`
    Version string `yaml:"version"`
}
```

**Defaults:** If `frank.yaml` is minimal (just `version: 1`), you get PHP 8.5 + FrankenPHP + pgsql + mailpit + latest Laravel.

**Validation:** Checks for valid PHP versions (8.2-8.5), valid service names, valid runtime options. Enforces single-database constraint (cannot have both pgsql and mysql). Replaces `validate.sh`.

**Project name:** Derived from the target directory basename (same as current `frank_project_name()`). Not stored in `frank.yaml`.

**Laravel version:** Supports `"latest"` (no version constraint), `"lts"` (maps to current LTS, e.g. `11.*`), or a specific version string (e.g. `"11.*"`).

## Template Engine (`internal/template`)

- Embedded templates use Go's native `{{.Var}}` syntax (replaces `%%..%%` Frankies)
- Go's `text/template` provides conditionals, loops, and pipelines natively
- No conflict with shell `${}` variables — Go templates are processed at generation time, not shell time
- Templates are read from `embed.FS` at runtime

## Compose Generation (`internal/compose`)

Builds `compose.yaml` programmatically using a hybrid approach:

- **Typed structs** for the parts Frank controls (services, volumes, networks)
- **Raw `map[string]interface{}`** for flexible/pass-through config

Generation flow:
1. Start with base compose structure (app service + runtime config)
2. Merge each configured service's compose fragment
3. Apply service-specific overrides from `config:` section in `frank.yaml`
4. Generate `.frank/env.generated` from per-service `env.yaml` metadata (DB credentials, ports, etc.)
5. Write final `compose.yaml` with header comment: `# Generated by Frank — edit frank.yaml, not this file`

### Naming Schemes

Two output modes for service naming:

| | Frank mode (default) | Sail mode (`--sail`) |
|---|---|---|
| App service | `app` | `laravel.test` |
| Database | `db` | `pgsql` / `mysql` / `mariadb` |
| Redis | `redis` | `redis` |
| etc. | Frank conventions | Sail conventions |

Sail mode is activated via `--sail` flag on `generate`, or implicitly by `init --sail` and `export`.

## Docker Wrapper (`internal/docker`)

Thin wrapper around `docker compose` via `os/exec`:

- Streams stdout/stderr directly to the terminal (no buffering)
- Passes through exit codes from Docker
- Operates on the `compose.yaml` in the target project directory
- **Early dependency check:** On any command that needs Docker, verify `docker` and `docker compose` are available and the daemon is running. Fail with a clear message if not.
- Bare `frank` calls `docker compose ps` quietly to detect state for smart help.

## Shell Integration (`internal/shell`)

### `frank activate`

Outputs aliases scoped to the current project's configured services:

```sh
alias artisan='docker compose exec app php artisan'
alias composer='docker compose exec app composer'
alias php='docker compose exec app php'
alias tinker='docker compose exec app php artisan tinker'
alias npm='docker compose exec app npm'
alias bun='docker compose exec app bun'
# Service-specific (only if configured):
alias psql='docker compose exec db psql ...'     # pgsql
alias mysql='docker compose exec db mysql ...'    # mysql/mariadb
alias redis-cli='docker compose exec redis redis-cli'  # redis
```

### `frank shell-setup`

Outputs a shell snippet for one-time addition to `.zshrc` or `.bashrc`:

```sh
eval "$(frank shell-setup)"
```

Registers a hook that:
1. On `cd` into a directory with `frank.yaml` → `eval "$(frank activate)"`
2. On `cd` out of a Frank directory → unsets all Frank aliases

**Shell detection:** Automatic. Outputs `chpwd` hook for zsh, `PROMPT_COMMAND` hook for bash.

**Supported shells:** zsh and bash. Fish is a future consideration.

## Smart Help (bare `frank`)

Contextual help based on project state:

**No `frank.yaml`:**
```
Frank — Laravel Development Environment

  No frank.yaml found in this directory.
  Run 'frank init' to get started.
```

**Config exists, containers stopped:**
```
Frank — Laravel Development Environment

  Project: my-app (frankenphp, php 8.5)
  Services: pgsql, mailpit
  Status: stopped

  → frank up    to start

Commands:
  init, generate, add, remove, install,
  up, down, ps, clean, reset,
  activate, shell-setup, import, export
```

**Config exists, containers running:**
```
Frank — Laravel Development Environment

  Project: my-app (frankenphp, php 8.5)
  Services: pgsql, mailpit
  Status: running (3/3 containers)

  → frank ps    for details
  → frank down  to stop

Commands:
  ...
```

## Reset Behavior

`frank reset` is a destructive operation. It:

1. Runs `docker compose down -v` (stops containers, removes volumes)
2. Prompts for confirmation (unless `--force` flag is passed)
3. Deletes all project files except a preserved set: `.git/`, `frank.yaml`, `.dockerignore`, `.gitignore`, `README.md`
4. Restores `.gitignore` from git if it was modified

The preserved file list may evolve — the implementation should keep it configurable internally.

## Install Behavior

`frank install` does more than run the Laravel installer:

1. Runs `composer create-project` inside a container (no local PHP needed)
2. Patches `.env` with values from `.frank/env.generated` (DB host, credentials, ports, etc.)
3. Modifies `vite.config.js` to set `server.host = '0.0.0.0'` for Docker HMR support
4. Backs up and restores `README.md` and `.gitignore` (Laravel installer overwrites these)
5. Copies `.psysh.php` for tinker configuration

## Add/Remove Validation

`frank add <service>`:
- Fails if the service already exists in `frank.yaml`
- Fails if adding a database when a different database is already configured (single-database constraint)
- Validates the service name is in the supported list
- Auto-runs `frank generate` after modifying `frank.yaml`

`frank remove <service>`:
- Fails if the service is not in `frank.yaml`
- Auto-runs `frank generate` after modifying `frank.yaml`

## Supported Services

Same 8 services as current Frank:

- **Databases:** pgsql, mysql, mariadb, sqlite
- **Cache:** redis, memcached
- **Search:** meilisearch
- **Mail:** mailpit

## Supported Runtimes

- **FrankenPHP** (default) — modern, single-binary PHP server
- **PHP-FPM** + nginx — traditional setup

## Supported PHP Versions

8.2, 8.3, 8.4, 8.5 (default: 8.5)

## Future Considerations

- Podman support (CLI-compatible with Docker, minimal changes in `internal/docker`)
- Fish shell support for `shell-setup`
- Homebrew tap distribution
- Additional services (e.g., MinIO, Soketi)
- `frank eject` — stop using Frank, keep generated files as-is
