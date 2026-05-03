<p align="center">
  <img src="docs/assets/logo.svg" alt="Frank" width="150"><br>
</p>

<h1 align="center">Frank</h1>

<p align="center">
  <a href="https://github.com/phlisg/frank"><img src="https://img.shields.io/github/go-mod/go-version/phlisg/frank" alt="Go Version"></a>&nbsp;
  <a href="https://github.com/phlisg/frank/releases"><img src="https://img.shields.io/github/v/release/phlisg/frank" alt="Release"></a>&nbsp;
  <a href="https://goreportcard.com/report/github.com/phlisg/frank"><img src="https://goreportcard.com/badge/github.com/phlisg/frank" alt="Go Report Card"></a>&nbsp;
  <a href="LICENSE"><img src="https://img.shields.io/github/license/phlisg/frank" alt="License"></a>
</p>

> A config-driven Docker environment for full-fledged Laravel development.

Frank gives you a full Laravel dev environment from a single `frank.yaml` — no local PHP, Composer, or Node required. Queue workers and the scheduler run as dedicated containers with auto-reload on code change. Onboard a teammate with `git clone` and `frank up`.

### Features

**Single file config**
- **One-file config** (`frank.yaml`) → generates Dockerfile, compose, Caddy/nginx 

**Flexible environments**:
- **Two runtimes**: FrankenPHP (default) or PHP-FPM + Nginx 
- **Services**: Postgres, MySQL, MariaDB, SQLite, Redis, Memcached, Meilisearch, Mailpit, and more

**Workflow**
- `frank new` scaffolds a project (interactive or flag-driven) 
- `frank install` bootstraps Laravel inside the container - Shell aliases (`artisan`, `composer`, `php`, `psql`, …) auto-activate on `cd` 
- **Custom project aliases** in `frank.yaml` (container or host-side) 
- Shell completion for zsh / bash / fish / powershell

**Automatically reloading workers**
- Declared `schedule:work` + `queue:work` pools in `frank.yaml` 
- Ad-hoc workers via `frank worker queue|schedule` 
- Host-side file watcher (`frank watch`) reloads workers on code change 
- Multi-pane CCTV view of every worker: `frank worker top`

**Dev Tools**
- **Preconfigured** _Pint_, _Larastan_, _Rector_  configuration with opinionated Laravel defaults 
- **_Lefthook_** pre-commit hooks: auto-fix on commit, analyze before push 
- `frank tool add <name>` to install tools on existing projects - `frank generate` reconciles tools for new devs cloning the repo$

**Interop**
- Import existing Laravel Sail projects (`frank import`)
- Hand off to Sail anytime (`frank eject`)
- Single static Go binary — no runtime dependencies

---

## Contents

- [Install](#install)
- [Getting Started](#getting-started)
- [frank.yaml](#frankyaml)
- [Supported Services](#supported-services)
- [CLI Commands](#cli-commands)
- [Further Reading](#further-reading)

---

## Install

### Homebrew (macOS & Linux)

```bash
brew install phlisg/tap/frank
```

No Go required. Updates via `brew upgrade frank`.

### Go install

If you have Go 1.26+ installed:

```bash
go install github.com/phlisg/frank@latest
```

> **Tip**: Don't have Go locally installed and the setup scares you? Try [Proto](https://moonrepo.dev/proto), installs Go in one command :)

<details>
<summary>Per-OS notes</summary>

**WSL (Windows)** — either method works. Make sure Docker Desktop has the **WSL 2 backend** enabled (Settings → Resources → WSL Integration).

**Tip:** for better Docker volume mount performance, enable VirtioFS in Docker Desktop → Settings → General → "Use VirtioFS for file sharing".

</details>

---

## Getting Started

```bash
frank new my-app
cd my-app
```

That's it. Frank scaffolds the project, installs Laravel, starts containers, and runs migrations. Visit [http://localhost](http://localhost).

No local PHP, Composer, or Node required.

### What just happened?

`frank new` walked you through PHP version, runtime, and services — or skip the wizard entirely:

```bash
frank new --php 8.4 --runtime frankenphp --with="pgsql,redis,mailpit" --schedule --queue-count 2 my-app
```

Frank generated `frank.yaml`, Dockerfile, compose, Caddy/nginx config, `.env` — then built and started everything.

### Shell aliases

```bash
echo 'eval "$(frank config shell setup)"' >> ~/.zshrc   # or ~/.bashrc
source ~/.zshrc
```

> **Note:** this line must appear **after** your Go bin path (`export PATH=$PATH:$HOME/go/bin` or equivalent) in your shell config — otherwise `frank` won't be found when the eval runs.

Now `artisan`, `composer`, `php`, `npm` and any custom alias resolve to the container automatically when you're in a Frank project:

```bash
artisan make:controller Api/PostController --resource
php vendor/bin/pint
composer require filament/filament
frank test
npm run dev
```

### Onboard a teammate

```bash
git clone … && cd my-app
frank generate
frank up -d
```

`frank generate` rebuilds the Docker files from `frank.yaml`. Then `frank up` starts containers, runs migrations — same environment, every machine.

---

## frank.yaml

`frank.yaml` is the single source of truth for your environment. All Docker files (`compose.yaml`, `Dockerfile`, `.env`, etc.) are generated from it. Commit `frank.yaml` to git; the generated files can be gitignored or committed alongside — your choice.

```yaml
version: 1

php:
  version: "8.5"
  runtime: "frankenphp"

laravel:
  version: "latest"

services:
  - pgsql
  - mailpit
```

| Key | Values | Default | Description |
| --- | ------ | ------- | ----------- |
| `php.version` | `8.2` `8.3` `8.4` `8.5` | `8.5` | PHP version |
| `php.runtime` | `frankenphp` `fpm` | `frankenphp` | Web server runtime |
| `laravel.version` | `latest` `lts` `12.*` `13.*` … | `latest` | Laravel version constraint passed to Composer |
| `services` | list — see table below | `[pgsql, mailpit]` | Services to include |
| `config.<service>.port` | integer | service default | Override the host-side port mapping |
| `workers.schedule` | boolean | `false` | Run `php artisan schedule:work` in a dedicated container |
| `workers.queue` | list — see [`docs/workers.md`](docs/workers.md) | `[]` | Declare long-running `queue:work` worker pools |
| `tools` | list — `pint` `larastan` `rector` `lefthook` | `[]` | Dev tools installed by `frank new` or `frank tool add` — see [`docs/tools.md`](docs/tools.md) |
| `server.https` | boolean | `true` | Serve over HTTPS with locally-trusted mkcert certificates — see [`docs/https.md`](docs/https.md) |
| `server.port` | integer | `443` (HTTPS) / `80` (HTTP) | Custom host-side port |
| `aliases` | map | `{}` | Custom shell aliases activated by `frank config shell activate` |
| `aliases.<name>` | string or `{cmd, host}` | — | String = container command; map with `host: true` = host-side |

After editing `frank.yaml`, run `frank generate` to regenerate Docker files — or simply `frank up`, which auto-regenerates when `frank.yaml` is newer than `compose.yaml`.

---

## Supported Services

| Service | Category | Default port |
| ------- | -------- | ------------ |
| `pgsql` | Database | 5432 |
| `mysql` | Database | 3306 |
| `mariadb` | Database | 3306 |
| `sqlite` | Database | — (file-based) |
| `redis` | Cache / Queue | 6379 |
| `meilisearch` | Search | 7700 |
| `memcached` | Cache | 11211 |
| `mailpit` | Mail (local SMTP + UI) | 1025 / 8025 |

Only one database can be active at a time. Frank enforces this — `frank add mysql` will refuse if `pgsql` is already configured. Use `frank remove pgsql` first.

Ports are customizable in `frank.yaml` via `config.<service>.port`:
```yaml
config:
  pgsql:
    port: 5433
  redis:
    port: 6380
```

---

## CLI Commands

| Command | Description |
| ------- | ----------- |
| `frank new <project>` | Create a new Laravel project — zero to localhost in one command. Non-interactive by default; use `--interactive` for wizard. Flags: `--php`, `--laravel`, `--runtime`, `--with`, `--schedule`, `--queue-count`, `--http`, `--no-pint`, `--no-larastan`, `--no-rector`, `--no-lefthook`, `--no-tools`, `--no-up`, `--sail` |
| `frank setup` | Configure Frank in an existing Laravel project (interactive wizard). Supports `--sail` and `--dir` |
| `frank tool add <tool>` | Add a dev tool to `frank.yaml` and install its config files |
| `frank generate` | Regenerate Docker files from `frank.yaml` without prompting |
| `frank install` | Install Laravel into the project directory |
| `frank add <service>` | Add a service to `frank.yaml` and regenerate |
| `frank remove <service>` | Remove a service from `frank.yaml` and regenerate |
| `frank up [-d] [--quick] [-- <compose args>]` | Start containers. Frank owns `-d/--detach` and `--quick`; all other docker compose flags must come after a literal `--` (e.g. `frank up -- --build`). Auto-spawns the watcher when workers are declared |
| `frank down` | Stop containers and the watcher. Use `frank down -- -v` to also remove volumes |
| `frank test [-- <artisan/pest flags>]` | Run tests inside the app container (`php artisan test`). Pest parallel works out of the box — see [`docs/testing.md`](docs/testing.md) |
| `frank exec <cmd> [args...]` | Run a command inside the app container as sail (e.g. `frank exec bash`, `frank exec php vendor/bin/pint`) |
| `frank compose [--] <args>` | Pass-through to `docker compose` (e.g. `frank compose ps`, `frank compose logs`) |
| `frank worker queue [--count N] [--queue …] [--tries …] [-- <artisan flags>]` | Spawn ad-hoc `queue:work` workers |
| `frank worker schedule` | Spawn an ad-hoc `schedule:work` container |
| `frank worker ps` | Show declared + ad-hoc worker containers |
| `frank worker stop [--all]` | Stop ad-hoc workers; `--all` also stops declared ones |
| `frank worker logs [name] [-f]` | Tail logs for one or all workers |
| `frank worker top [--live] [--min-pane-width N]` | Live multi-pane CCTV view of every worker; `--live` reconciles ad-hoc churn |
| `frank watch [--status\|--stop]` | Run the code-reload watcher in the foreground, or inspect/stop the detached one |
| `frank config show` | Show resolved configuration — see [`docs/config.md`](docs/config.md) |
| `frank config edit` | Open frank.yaml in your editor — see [`docs/config.md`](docs/config.md) |
| `frank config set <key> <value>` | Set a config value (e.g. `frank config set php.version 8.4`) — see [`docs/config.md`](docs/config.md) |
| `frank config shell ...` | Shell integration (aliases, hooks, completion) — see [`docs/shell.md`](docs/shell.md) |
| `frank import [-f path]` | Import from a Sail `docker-compose.yml` |
| `frank eject` | Install Laravel Sail into the running containers and hand off to Sail |
| `frank version [--check\|--update]` | Print version and check for updates. `--check` shows update status; `--update` self-updates via Homebrew or `go install` |

---

## Further Reading

- HTTPS (local TLS) — [`docs/https.md`](docs/https.md)
- Testing — [`docs/testing.md`](docs/testing.md)
- Dev tools — [`docs/tools.md`](docs/tools.md)
- Workers & code reload — [`docs/workers.md`](docs/workers.md)
- Project and PHP tools — [`docs/tools.md`](docs/tools.md)
- PHP runtimes — [`docs/runtimes.md`](docs/runtimes.md)
- Shell aliases — [`docs/shell.md`](docs/shell.md)
- Sail interop — [`docs/sail-interop.md`](docs/sail-interop.md)
- Contributing — [`docs/contributing.md`](docs/contributing.md)
