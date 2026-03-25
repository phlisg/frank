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
| `frank install` | Runs Laravel installer inside a container (see Install Behavior below) |

### Service Management

| Command | Description |
|---|---|
| `frank add <service>` | Add a service to `frank.yaml` and regenerate |
| `frank remove <service>` | Remove a service from `frank.yaml` and regenerate |

### Lifecycle (Docker Compose wrappers)

| Command | Description |
|---|---|
| `frank up` | `docker compose up -d --build`, then runs post-start tasks (see Up Behavior below) |
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
| `frank export [path]` | Export current Frank project files to a target path (already Sail-compatible since Frank uses Sail naming natively) |

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
4. Generate `.env` and `.env.example` with service-aware values (see Environment Generation)
5. Write final `compose.yaml` with header comment: `# Generated by Frank — edit frank.yaml, not this file`

### Service Naming

Frank uses Sail-compatible service names natively. No dual naming scheme — one set of names everywhere:

| Service | Compose service name |
|---|---|
| App | `laravel.test` |
| PostgreSQL | `pgsql` |
| MySQL | `mysql` |
| MariaDB | `mariadb` |
| Redis | `redis` |
| Memcached | `memcached` |
| Meilisearch | `meilisearch` |
| Mailpit | `mailpit` |

This means generated `compose.yaml` files are Sail-compatible out of the box. `frank export` simply copies the already-compatible files to a target path. No branching, no naming toggles.

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
# Core aliases (always set):
alias artisan='docker compose exec laravel.test php artisan'
alias composer='docker compose exec laravel.test composer'
alias php='docker compose exec laravel.test php'
alias tinker='docker compose exec laravel.test php artisan tinker'
alias npm='docker compose exec laravel.test npm'
alias bun='docker compose exec laravel.test bun'
# Service-specific (only if configured):
alias psql='docker compose exec pgsql psql -U ${DB_USERNAME:-sail} -d ${DB_DATABASE:-laravel}'  # pgsql
alias mysql='docker compose exec mysql mysql -u ${DB_USERNAME:-sail} -p${DB_PASSWORD:-password} ${DB_DATABASE:-laravel}'  # mysql
alias mariadb='docker compose exec mariadb mariadb -u ${DB_USERNAME:-sail} -p${DB_PASSWORD:-password} ${DB_DATABASE:-laravel}'  # mariadb
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

## Up Behavior

`frank up` aims to get the developer to a working state in one command:

1. `docker compose up -d --build` — start all containers
2. `composer install` — ensure dependencies are in sync (handles post-`git pull` or branch switch)
3. `php artisan migrate` — run pending migrations

This covers the most common "forgot to run X" scenarios without being intrusive. Post-start tasks fail gracefully (logged but don't abort `frank up`) since the containers are already running.

**`frank up --quick`** — skips post-start tasks, only boots containers.

**Default ports:** 80 (HTTP) and 443 (HTTPS). FrankenPHP/Caddy serves directly on standard ports. Port 8000 is reserved for future Podman support where binding to privileged ports may require extra config.

**Out of scope for v1:** Auto-detecting git branch changes to determine if `composer install` or migrations are needed. The unconditional run on `frank up` is simple and covers this case already.

## Reset Behavior

`frank reset` is a destructive operation. It:

1. Runs `docker compose down -v` (stops containers, removes volumes)
2. Prompts for confirmation (unless `--force` flag is passed)
3. Deletes all project files except a preserved set: `.git/`, `frank.yaml`, `.dockerignore`, `.gitignore`, `README.md`
4. Restores `.gitignore` from git if it was modified

The preserved file list may evolve — the implementation should keep it configurable internally.

## Environment Generation

Frank generates the project `.env` file from scratch — it does not merely patch Laravel's default `.env.example`. The generated `.env` uses Sail-compatible defaults and is adapted based on configured services:

**Base values:** Standard Laravel defaults (APP_NAME, APP_KEY, APP_URL, etc.)

**Service-specific overrides:**

| Service | `.env` values set |
|---|---|
| pgsql | `DB_CONNECTION=pgsql`, `DB_HOST=pgsql`, `DB_PORT=5432`, `DB_DATABASE=laravel`, `DB_USERNAME=sail`, `DB_PASSWORD=password`, `DB_SSLMODE=prefer`, `DB_URL=postgresql://...` |
| mysql | `DB_CONNECTION=mysql`, `DB_HOST=mysql`, `DB_PORT=3306`, `DB_DATABASE=laravel`, `DB_USERNAME=sail`, `DB_PASSWORD=password` |
| mariadb | `DB_CONNECTION=mariadb`, `DB_HOST=mariadb`, `DB_PORT=3306`, `DB_DATABASE=laravel`, `DB_USERNAME=sail`, `DB_PASSWORD=password` |
| sqlite | `DB_CONNECTION=sqlite` (no host/port/credentials) |
| redis | `REDIS_HOST=redis`, `CACHE_STORE=redis`, `SESSION_DRIVER=redis`, `QUEUE_CONNECTION=redis` |
| memcached | `MEMCACHED_HOST=memcached`, `CACHE_STORE=memcached` |
| meilisearch | `SCOUT_DRIVER=meilisearch`, `MEILISEARCH_HOST=http://meilisearch:7700`, `MEILISEARCH_NO_ANALYTICS=false` |
| mailpit | `MAIL_MAILER=smtp`, `MAIL_HOST=mailpit`, `MAIL_PORT=1025`, `MAIL_FROM_ADDRESS=hello@example.com` |

The values above are baseline examples. The actual env variables for each service should be derived from Laravel's config files (`config/database.php`, `config/cache.php`, `config/mail.php`, `config/scout.php`, etc.) to stay in sync with what Laravel expects. During implementation, read the latest Laravel config to determine which env variables each driver references, including newer additions like `DB_SSLMODE` and `DB_URL`.

Host names match the Docker Compose service names (Sail convention). Credentials use Sail defaults for familiarity.

Frank also generates `.env.example` with the same structure but placeholder values, so new team members can see what's expected.

## Install Behavior

`frank install` does more than run the Laravel installer:

1. Runs `composer create-project` inside a container (no local PHP needed)
2. Generates `.env` from Frank's service-aware template (replaces Laravel's default `.env`)
3. Generates `.env.example` with matching structure
4. Modifies `vite.config.js` to set `server.host = '0.0.0.0'` for Docker HMR support
5. Backs up and restores `README.md` and `.gitignore` (Laravel installer overwrites these)
6. Copies `.psysh.php` for tinker configuration

## Add/Remove Validation

`frank add <service>`:
- Fails if the service already exists in `frank.yaml`
- Fails if adding a database when a different database is already configured — only one of `pgsql`, `mysql`, `mariadb`, `sqlite` is allowed at a time (Laravel supports a single `DB_CONNECTION`)
- Validates the service name is in the supported list (pgsql, mysql, mariadb, sqlite, redis, memcached, meilisearch, mailpit)
- Auto-runs `frank generate` after modifying `frank.yaml`

`frank remove <service>`:
- Fails if the service is not in `frank.yaml`
- Auto-runs `frank generate` after modifying `frank.yaml`

Non-database services (redis, memcached, meilisearch, mailpit) have no mutual conflicts and can coexist freely.

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
