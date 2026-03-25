# Frank Flexibility Design Spec

> **Date:** 2026-03-25
> **Status:** Draft
> **Goal:** Evolve Frank from a hardcoded Docker-based Laravel dev environment into a flexible, config-driven Sail alternative.

## Overview

Frank is a standalone tool that generates and manages Docker-based Laravel development environments. It requires no local PHP, Node, or Composer installation — only Docker and Just (later: a single Go binary).

Frank follows the Laravel philosophy: sensible defaults that just work, with full configurability when needed.

## Host Dependencies

The following must be installed on the host machine:

| Tool | Required | Notes |
|---|---|---|
| Docker (with Compose v2) | Yes | The only runtime dependency |
| [just](https://just.systems) | Yes (for now) | Task runner. Replaced by Go binary later. |
| `yq` | Yes (for now) | YAML processing for template merging. Replaced by Go binary later. |

**Future state:** A single Go binary replaces both Just and yq, leaving Docker as the only dependency.

## Target Audience

Start personal, grow public. Design decisions should be opinionated but extensible — optimize for the author's workflow now, but architect for community adoption later.

## Core Principles

1. **Config-driven:** `frank.yaml` is the single source of truth.
2. **Sensible defaults:** Zero-config produces a working environment (PHP 8.5, FrankenPHP, pgsql + mailpit, latest Laravel).
3. **Tool, not template:** Frank generates projects into target directories. Templates live in Frank, not in the user's project. Ready for Go binary embedding via `embed.FS` later.
4. **Sail-compatible:** Import from and export to Sail's docker-compose format.
5. **Venv-inspired:** Shell aliases make Docker-wrapped tools feel native.

## Config File Schema (`frank.yaml`)

```yaml
# frank.yaml
version: 1

php:
  version: "8.5"           # 8.2, 8.3, 8.4, 8.5
  runtime: "frankenphp"     # frankenphp, fpm

laravel:
  version: "latest"         # latest, lts, or specific (e.g. "11.*")

services:
  - pgsql
  - mailpit

# Service-specific overrides (optional)
config:
  pgsql:
    port: 5432
    version: "latest"
  mailpit:
    port: 1025
    dashboard_port: 8025
```

**Defaults when fields are omitted:**

| Field | Default |
|---|---|
| `php.version` | `8.5` |
| `php.runtime` | `frankenphp` |
| `laravel.version` | `latest` |
| `services` | `[pgsql, mailpit]` |

## Supported Services (v1)

| Service | Category | Default Port(s) | Conflicts With |
|---|---|---|---|
| `pgsql` | Database | 5432 | `mysql`, `mariadb` |
| `mysql` | Database | 3306 | `pgsql`, `mariadb` |
| `mariadb` | Database | 3306 | `pgsql`, `mysql` |
| `sqlite` | Database | N/A (no container) | — |
| `redis` | Cache/Queue | 6379 | — |
| `meilisearch` | Search | 7700 | — |
| `memcached` | Cache | 11211 | — |
| `mailpit` | Mail | 1025, 8025 | — |

**Conflict rules:** Only one primary database container at a time (`pgsql`, `mysql`, or `mariadb` — pick one). `sqlite` does not conflict with anything since it requires no container; it can coexist with a container-based DB (e.g., SQLite as default connection + Postgres for specific tests). Cache, search, and mail services can coexist freely.

**Database container naming:** The primary database service is always named `db` in the generated compose file, regardless of which engine is chosen. This keeps aliases, env vars, and `depends_on` consistent.

## Supported PHP Runtimes

| Runtime | Containers | Base Image | Server Config | Notes |
|---|---|---|---|---|
| `frankenphp` | 1 (`app`) | `dunglas/frankenphp:1-php{version}` | `Caddyfile` | Default. Single container with built-in web server. |
| `fpm` | 2 (`app` + `nginx`) | `php:{version}-fpm` + `nginx:alpine` | `nginx.conf` | Traditional setup. Matches shared hosting environments. |

### FPM runtime architecture

The `fpm` runtime generates **two containers** in the compose file:

- **`app`** — the PHP-FPM container (handles PHP execution)
- **`nginx`** — an Nginx reverse proxy that forwards `.php` requests to `app:9000`

Both share the `/app` volume. The `nginx` service depends on `app` being healthy. This is transparent to the user — aliases like `php`, `artisan`, `composer` still target the `app` container.

The `fpm` runtime template includes:
- `Dockerfile.tmpl` — for the PHP-FPM container
- `nginx.Dockerfile.tmpl` — for the Nginx container (minimal, just copies config)
- `nginx.conf.tmpl` — Nginx configuration with Laravel-friendly rewrite rules

### PHP version and Docker image tags

- **FrankenPHP:** uses `dunglas/frankenphp:1-php{version}` tags (e.g., `dunglas/frankenphp:1-php8.5`). The PHP version in `frank.yaml` maps directly to the image tag.
- **FPM:** uses `php:{version}-fpm` tags (e.g., `php:8.5-fpm`).

If a requested PHP version is not available as a Docker image tag, the generator warns and falls back to the latest available version.

## Template Architecture

Templates live within Frank (the tool), not the generated project.

```
frank-source/                          # The Frank tool repo (cloned or installed)
  frank/
    templates/
      services/
        pgsql/
          compose.yaml                 # docker-compose fragment
          env.yaml                     # .env variables to inject
          meta.yaml                    # metadata: name, category, ports, conflicts
        mysql/
        mariadb/
        sqlite/
          env.yaml                     # DB_CONNECTION=sqlite, no compose.yaml (no container)
          meta.yaml
        redis/
        meilisearch/
        memcached/
        mailpit/
      runtimes/
        frankenphp/
          Dockerfile.tmpl
          Caddyfile.tmpl
          compose.yaml                 # app service definition for frankenphp
        fpm/
          Dockerfile.tmpl
          nginx.Dockerfile.tmpl        # minimal Dockerfile for nginx container
          nginx.conf.tmpl
          compose.yaml                 # app + nginx service definitions for fpm
      base/
        compose.yaml                   # base structure (networks, volumes header)
      activate.tmpl                    # shell alias template
    scripts/                           # Frank's own scripts (the tool logic)
      init.sh                          # interactive wizard + frank.yaml writer
      generate.sh                      # reads frank.yaml, renders templates
      install.sh                       # Laravel project creation
      add.sh                           # add service to frank.yaml + regenerate
      remove.sh                        # remove service from frank.yaml + regenerate
      sail-import.sh                   # Sail → frank.yaml parser
      sail-export.sh                   # frank.yaml → Sail compose
    justfile.tmpl                      # template for the generated justfile
```

**Key distinction:** `frank-source/frank/scripts/` contains Frank's tool logic (the generator, wizard, etc.). These scripts are NOT copied into the user's project. The generated project's `.frank/scripts/` only contains runtime helpers (activate, shell-setup, psysh config).

**SQLite special case:** The `sqlite/` template has `env.yaml` and `meta.yaml` but no `compose.yaml`. The generator skips compose merging for services that lack a `compose.yaml` fragment — it only applies their `env.yaml` entries.

### Service template structure

**`meta.yaml` example (pgsql):**

`meta.yaml` is metadata for validation and the wizard UI only — it is NOT used in generation output. Healthchecks, image names, and ports in `meta.yaml` are for validation and display. The `compose.yaml` fragment is the source of truth for generated output.

```yaml
name: PostgreSQL
category: database
default_port: 5432
conflicts: [mysql, mariadb]
env_prefix: DB_
image: postgres
container_name: db            # always "db" for databases
```

**`env.yaml` example (pgsql):**

```yaml
# Key-value pairs to inject into .env
# %%...%% is resolved by the generator at generation time ("Frankies")
DB_CONNECTION: pgsql
DB_HOST: db
DB_PORT: "%%config.pgsql.port:-5432%%"
DB_DATABASE: "%%project_name%%"
DB_USERNAME: root
DB_PASSWORD: root
```

**`env.yaml` rules:**
- Simple key-value pairs. Values support `%%config.<service>.<key>:-default%%` interpolation from `frank.yaml` config overrides, and `%%project_name%%` (the directory name).
- Entries are merged into `.env`. If `.env` doesn't exist, it's created from `.env.example` first.
- Conflicting keys across services: last-listed service in `frank.yaml` wins. In practice, only one database is active, so DB_ keys don't conflict.

### Template interpolation syntax

Two distinct syntaxes exist in templates, resolved at different times:

| Syntax | Resolved by | When | Example |
|---|---|---|---|
| `%%variable%%` | Frank's generator ("Frankies") | At generation time | `%%config.pgsql.port:-5432%%`, `%%project_name%%`, `%%php.version%%` |
| `${VARIABLE}` | Docker Compose | At container runtime | `${DB_DATABASE}`, `${DB_USERNAME}` |

**Rule:** `%%...%%` (double percent, aka "Frankies") are Frank template variables — the generator replaces them with concrete values when producing output files. `${...}` (dollar-brace) are standard Docker Compose / shell variable references that survive generation and are resolved at runtime from `.env`. This syntax was chosen to avoid conflicts with Just's `{{}}` interpolation and Caddy's `{path}` syntax.

This means the generator never needs to distinguish "resolve now" vs. "leave for runtime" — the syntax makes it unambiguous.

**`compose.yaml` fragment example (pgsql):**

```yaml
# This fragment is merged into the generated compose.yaml
# %%...%% is resolved at generation time, ${...} is resolved by Docker Compose at runtime
db:
  image: "postgres:%%config.pgsql.version:-latest%%"
  environment:
    POSTGRES_DB: "${DB_DATABASE}"
    POSTGRES_USER: "${DB_USERNAME}"
    POSTGRES_PASSWORD: "${DB_PASSWORD}"
  volumes:
    - db_data:/var/lib/postgresql/data
  ports:
    - "%%config.pgsql.port:-5432%%:5432"
  networks:
    - frank
  healthcheck:
    test: ["CMD-SHELL", "pg_isready -U ${DB_USERNAME:-root}"]
    interval: 5s
    timeout: 3s
    retries: 10
```

### Volume and network naming conventions

- **Network:** always `frank` (single bridge network)
- **Volumes:** `{service}_data` pattern (e.g., `db_data`, `redis_data`, `meilisearch_data`)
- **Container/service names:** match the service key from `frank.yaml` except databases, which are always `db`

### Runtime compose fragments

Each runtime directory includes a `compose.yaml` that defines the `app` service (and `nginx` for FPM). This is the source of truth for how the main application container is configured.

**`runtimes/frankenphp/compose.yaml`** defines:
- The `app` service with the FrankenPHP image, port mappings (8000, 5173, 2019), volumes, environment, and `depends_on` for the database

**`runtimes/fpm/compose.yaml`** defines:
- The `app` service with the PHP-FPM image, shared `/app` volume, and `depends_on` for the database
- The `nginx` service with `nginx:alpine`, port mappings (8000, 5173), shared `/app` volume, and `depends_on: app`

These runtime compose fragments are merged into the output alongside the service fragments.

### Generation pipeline

1. Read `frank.yaml` (apply defaults for omitted fields)
2. Validate: check for service conflicts, unknown services, port uniqueness across all services
3. Start with `base/compose.yaml` (defines the `frank` network and volume declarations)
4. Select runtime from `runtimes/{frankenphp,fpm}/` — render `Dockerfile` (and `Caddyfile` or `nginx.conf` + nginx Dockerfile)
5. For each service in the list, read its `compose.yaml` fragment, interpolate config values, and merge into the output using `yq`
6. Collect all `env.yaml` entries, interpolate values, and prepare `.env` content
7. Render `activate.tmpl` with the appropriate aliases for selected services
8. Write output files to target directory

**YAML merging strategy:** Service compose fragments are merged into the base compose file using `yq` (a lightweight YAML processor). Each fragment is a valid YAML snippet that gets deep-merged into the `services:` key. This avoids fragile string concatenation and handles indentation correctly. When Frank moves to Go, this is replaced by native YAML marshalling.

**Port uniqueness validation:** The generator checks all exposed host ports across all services (including user overrides in the `config` block) and fails with a clear error if duplicates are found.

### Laravel installation pipeline

Separate from file generation. Triggered by `just install` after `just init` + `just generate`.

1. Read `frank.yaml` for Laravel version and database choice
2. Spin up a disposable `composer:latest` container (same as current `laravel-init` service)
3. Run `composer create-project laravel/laravel .temp-laravel {version}` where `{version}` comes from `frank.yaml`:
   - `latest` = no version constraint (Composer picks the newest)
   - `lts` = resolved via a hardcoded mapping in Frank (e.g., `lts` → `11.*` as of March 2026). This mapping is updated when Frank itself is updated. No network query beyond what Composer already does.
   - Specific version (e.g., `11.*`) = passed directly to Composer
4. Copy files to the project directory
5. Configure `.env` based on collected `env.yaml` entries (DB connection, host, port, credentials)
6. Configure `vite.config.js` for HMR
7. Clean up temp directory

**Node.js tooling:** Node, npm, pnpm, and bun are installed inside the `app` container's Dockerfile (as they are today). The current standalone `node` service is removed — it was only used for `npm install` which now runs in the app container's entrypoint. This simplifies the architecture without losing functionality.

## Generated Project Structure

After running `frank init` in an empty directory:

```
myproject/
  frank.yaml              # config (authored, version controlled)
  compose.yaml            # generated
  Dockerfile              # generated (app container)
  Caddyfile               # generated (frankenphp runtime only)
  nginx.Dockerfile        # generated (fpm runtime only)
  nginx.conf              # generated (fpm runtime only)
  justfile                # generated (thin wrapper, delegates to frank tool scripts)
  .frank/                 # Frank runtime files (generated)
    scripts/
      activate            # generated, dynamic per services
      shell-setup
    .psysh.php
```

**FPM runtime** produces `Dockerfile` (PHP-FPM) + `nginx.Dockerfile` + `nginx.conf` instead of `Dockerfile` + `Caddyfile`.

**Generated files** include a header comment: `# Generated by Frank — edit frank.yaml instead`.

**Recommendation:** Commit generated files by default for transparency. Users who prefer clean repos can gitignore them.

## CLI Commands (Just recipes)

| Command | Description |
|---|---|
| `just init` | Interactive wizard. Asks PHP version, runtime, services. Writes `frank.yaml` + generates Docker files. Can run in an empty directory. |
| `just init --from-sail [-f path]` | Reads a Sail `docker-compose.yml` (default: `./docker-compose.yml`) and produces a `frank.yaml`. |
| `just generate` | Regenerates `compose.yaml`, `Dockerfile`, and server config from `frank.yaml`. |
| `just generate -f path` | Generate to a specific output path. |
| `just install` | Creates the Laravel project. Reads `frank.yaml` for Laravel version and DB choice. |
| `just add <service>` | Adds a service to `frank.yaml` and regenerates. |
| `just remove <service>` | Removes a service from `frank.yaml` and regenerates. |
| `just up` | Start containers, run migrations, activate aliases. |
| `just down` | Stop containers, deactivate aliases. |
| `just clean` | Stop containers, remove volumes. |
| `just reset` | Runs `just clean` first (stops containers, removes volumes), then deletes generated and Laravel files, keeps only `frank.yaml` + `.git` + `.frank/`. Runs `just generate` afterward to restore generated Docker files from `frank.yaml`. |
| `just export-sail [-f path]` | Generate a Sail-compatible `docker-compose.yml` from `frank.yaml`. Default output: `./docker-compose.yml`. |

### Argument handling

Just does not support GNU-style flags (`--from-sail`, `-f`). All arguments are **positional** in Just recipes, with shell scripts handling the flag parsing. The actual Just recipe signatures are:

The generated `justfile` delegates to Frank's tool scripts. The `FRANK_HOME` variable points to where Frank is installed (the cloned repo or install path):

```just
# Path to Frank tool installation
FRANK_HOME := env_var_or_default("FRANK_HOME", "~/.frank")

# Interactive wizard (no args) or import from Sail
init *ARGS:
    {{FRANK_HOME}}/frank/scripts/init.sh {{ARGS}}

# Regenerate from frank.yaml
generate *ARGS:
    {{FRANK_HOME}}/frank/scripts/generate.sh {{ARGS}}

# Add a service
add SERVICE:
    {{FRANK_HOME}}/frank/scripts/add.sh {{SERVICE}}

# Remove a service
remove SERVICE:
    {{FRANK_HOME}}/frank/scripts/remove.sh {{SERVICE}}

# Export to Sail format
export-sail *ARGS:
    {{FRANK_HOME}}/frank/scripts/sail-export.sh {{ARGS}}
```

The shell scripts (`init.sh`, `generate.sh`, etc.) parse flags like `--from-sail` and `-f` using standard `getopts` or manual parsing. This keeps the Just layer thin and the logic in portable shell scripts (easing the Go migration).

**Examples:**
- `just init` — interactive wizard
- `just init --from-sail` — import from `./docker-compose.yml`
- `just init --from-sail -f /path/to/compose/file` — import from specified path
- `just generate -f /path/to/output` — generate to specified path
- `just export-sail -f /path/to/output` — export to specified path
- All default to the current directory with standard filenames when omitted

### Regeneration triggers

- `just generate` is the explicit regeneration command. Users run it after manually editing `frank.yaml`.
- `just add <service>` and `just remove <service>` modify `frank.yaml` and call `just generate` automatically.
- `just up` checks if generated files are **older than `frank.yaml`** (simple `stat` comparison). If stale, it runs `just generate` before starting containers and prints a notice: `"frank.yaml changed — regenerating..."`.
- `just init` always generates after writing `frank.yaml`.

## Sail Interop

### Import (`just init --from-sail`)

Parses a Sail `docker-compose.yml` and extracts:
- PHP version from Sail image tags (e.g., `sail-8.3/app` → PHP 8.3)
- Services from service names (Sail uses the same naming: `mysql`, `pgsql`, `redis`, etc.)
- Port mappings from `ports` declarations
- Environment variables from `environment` and `env_file`

**Limitations:**
- Custom Sail Dockerfile modifications don't transfer (user is warned)
- Sail's `context: ./vendor/laravel/sail/runtimes/8.3` paths are parsed for version info but not reused

### Export (`just export-sail`)

Reads `frank.yaml` and generates a Sail-style `docker-compose.yml`:
- Replaces Frank's runtime with `sail-{version}/app` image references
- Maps services to Sail's naming and structure conventions
- Best-effort — user may need `sail:install` afterward for full Sail vendor setup

## Activate/Deactivate System

The venv-inspired shell alias system becomes **dynamically generated** based on `frank.yaml`.

### Core aliases (always present)

| Alias | Maps to |
|---|---|
| `php` | `docker compose exec app php` |
| `composer` | `docker compose exec app composer` |
| `artisan` | `docker compose exec app php artisan` |
| `tinker` | `docker compose exec app php artisan tinker` |
| `npm` | `docker compose exec app npm` |
| `bun` | `docker compose exec app bun` |

### Service-specific aliases

| Service | Alias | Maps to |
|---|---|---|
| `pgsql` | `psql` | `docker compose exec db psql ...` |
| `mysql` / `mariadb` | `mysql` | `docker compose exec db mysql ...` |
| `redis` | `redis-cli` | `docker compose exec redis redis-cli` |

### Future enhancement: auto-activation

When Frank detects it's in a Frank directory (presence of `frank.yaml`), aliases could be automatically loaded — similar to `direnv` with `.envrc` or `nvm` with `.nvmrc`. Out of scope for v1, but the architecture supports it.

## Implementation Priority

1. **Selectable services** — template architecture, generator, `frank.yaml` schema
2. **Selectable PHP version/runtime** — runtime templates, Dockerfile generation
3. **Selectable Laravel version** — `laravel-init.sh` updates, version pinning
4. **Sail interop** — import/export scripts
5. **Go migration** — rewrite generator as a Go binary with embedded templates (future)

## Tooling Strategy

**Just + shell for now.** The generator and all CLI commands are implemented as Just recipes calling shell scripts. This allows fast iteration while the feature set stabilizes.

**Go later.** Once the config schema, template format, and generation logic are stable, port to a Go binary. Templates embed via `embed.FS`. The Go binary replaces both Just and the shell scripts, becoming the only dependency besides Docker.

## File Naming

Generated output uses `compose.yaml` (the modern Docker Compose convention, not `docker-compose.yml`). Sail interop reads/writes `docker-compose.yml` since that's what Sail uses. This distinction is intentional.

## Generated Justfile

The generated `justfile` in a Frank project is a **thin wrapper** that delegates to Frank's shell scripts. It does not contain business logic — all generation, validation, and orchestration logic lives in the scripts under `.frank/scripts/`. When Frank becomes a Go binary, the justfile is replaced by direct `frank` CLI commands (or the binary generates a justfile that calls itself).

## Migration from Current Frank

For existing Frank projects (pre-config-driven):

1. Run `just init` in the existing project — the wizard detects existing `docker-compose.yml` and offers to import settings (PHP version from Dockerfile, services from compose file)
2. Generated files (`compose.yaml`, `Dockerfile`, `Caddyfile`) replace the old hand-authored ones
3. The `frank/` directory (current) becomes `.frank/` (dot-prefixed, runtime files only)
4. The old `justfile` is replaced by a generated wrapper

This is a one-time migration. The init wizard handles it gracefully by detecting the presence of existing Frank infrastructure files.
