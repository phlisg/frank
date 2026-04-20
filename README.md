# Frank

> A config-driven Docker environment for Laravel development.

Frank gives you a full Laravel dev environment from a single `frank.yaml` —
no local PHP, Composer, or Node required. Queue workers and the scheduler
run as dedicated containers with auto-reload on code change. Onboard a
teammate with `git clone` and `frank up`.

### Features

**Environment**
- One-file config (`frank.yaml`) → generates Dockerfile, compose, Caddy/nginx
- Two runtimes: FrankenPHP (default) or PHP-FPM + Nginx
- Services: Postgres, MySQL, MariaDB, SQLite, Redis, Memcached, Meilisearch, Mailpit

**Workflow**
- `frank init` scaffolds a project (interactive or flag-driven)
- `frank install` bootstraps Laravel inside the container
- Shell aliases (`artisan`, `composer`, `php`, `psql`, …) auto-activate on `cd`
- Shell completion for zsh / bash / fish / powershell

**Workers**
- Declared `schedule:work` + `queue:work` pools in `frank.yaml`
- Ad-hoc workers via `frank worker queue|schedule`
- Host-side file watcher (`frank watch`) reloads workers on code change
- Multi-pane CCTV view of every worker: `frank worker top`

**Dev Tools**
- Preconfigured Pint, Larastan, Rector with opinionated Laravel defaults
- Lefthook pre-commit hooks: auto-fix on commit, analyse before push
- `frank tool add <name>` to install tools on existing projects
- `frank generate` reconciles tools for new devs cloning the repo

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

The preferred way to install Frank is via `go install` (requires Go 1.26+):

```bash
go install github.com/phlisg/frank@latest
```

<details>
<summary>Per-OS notes</summary>

**Linux** — alternatively, download a pre-built binary from [GitHub Releases](https://github.com/phlisg/frank/releases):

```bash
curl -Lo frank https://github.com/phlisg/frank/releases/latest/download/frank-linux-amd64
chmod +x frank
sudo mv frank /usr/local/bin/
```

**macOS** — `go install` is the only supported method. Pre-built binaries are unsigned and macOS Gatekeeper will quarantine them.

**WSL (Windows)** — the Linux binary works as-is, or use `go install`. Make sure Docker Desktop has the **WSL 2 backend** enabled (Settings → Resources → WSL Integration).

**Tip:** for better Docker volume mount performance, enable VirtioFS in Docker Desktop → Settings → General → "Use VirtioFS for file sharing".

</details>

---

## Getting Started

A full walkthrough from zero to a running Laravel app. Scenario: new project with PostgreSQL, Redis (cache + queues), and Mailpit.

**1. Scaffold the project**

```bash
frank init my-app
cd my-app
```

The wizard asks for PHP version, Laravel version, runtime, and services. Prefer flags? Skip every prompt in one shot:

```bash
frank init --php 8.4 --laravel 12 --runtime frankenphp --with="pgsql,redis,mailpit" my-app
```

Either way, Frank writes `frank.yaml` and generates `compose.yaml`, `Dockerfile`, `Caddyfile`, `.env`, and `.env.example`.

**2. Install Laravel**

```bash
frank install
```

Spins up a disposable `composer:latest` container, creates a fresh Laravel project, moves the files into your directory, and patches Vite for Docker HMR. No local PHP required.

**3. Start containers**

```bash
frank up -d
```

Starts all services in the background, runs `composer install`, and runs `php artisan migrate`. Visit [http://localhost](http://localhost) — you should see the Laravel welcome page.

**4. Enable shell aliases (once)**

```bash
eval "$(frank shell-setup)" >> ~/.zshrc   # or ~/.bashrc
source ~/.zshrc
```

Aliases auto-activate when you `cd` into a Frank project. Full alias table: [`docs/shell.md`](docs/shell.md).

**5. Day-to-day**

```bash
artisan make:controller Api/PostController --resource
artisan migrate:fresh --seed
npm run dev                         # Vite HMR on http://localhost:5173
```

Visit [http://localhost:8025](http://localhost:8025) for the Mailpit inbox — any mail your app sends in local dev lands here.

**6. Onboard a teammate**

```bash
git clone ...
cd my-app
frank up -d        # containers start, migrate runs
```

No local PHP, no Composer, no version conflicts.

**7. Queue workers & scheduler**

Declare them at init time:

```bash
frank init --schedule --queue-count 2 my-app
```

`frank up` auto-spawns `frank watch` so edits to `app/`, `config/`, `routes/`, etc. reload workers automatically. Details: [`docs/workers.md`](docs/workers.md).

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
| `tools` | list — `pint` `larastan` `rector` `lefthook` | `[]` | Dev tools installed by `frank init` or `frank tool add` — see [`docs/tools.md`](docs/tools.md) |

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

---

## CLI Commands

| Command | Description |
| ------- | ----------- |
| `frank init [dir]` | Interactive wizard — creates `frank.yaml` and generates Docker files; if `dir` is given, creates and targets that directory. Flags `--php`, `--laravel`, `--runtime`, `--with`, `--schedule`, `--queue-count`, `--no-pint`, `--no-larastan`, `--no-rector`, `--no-lefthook`, `--no-tools` skip the corresponding prompts for non-interactive use |
| `frank tool add <tool>` | Add a dev tool to `frank.yaml` and install its config files |
| `frank generate` | Regenerate Docker files from `frank.yaml` without prompting |
| `frank install` | Install Laravel into the project directory |
| `frank add <service>` | Add a service to `frank.yaml` and regenerate |
| `frank remove <service>` | Remove a service from `frank.yaml` and regenerate |
| `frank up [-d] [--quick] [-- <compose args>]` | Start containers. Frank owns `-d/--detach` and `--quick`; all other docker compose flags must come after a literal `--` (e.g. `frank up -- --build`). Auto-spawns the watcher when workers are declared |
| `frank down` | Stop containers and the watcher |
| `frank ps` | Show running containers |
| `frank clean` | Stop containers and delete volumes |
| `frank worker queue [--count N] [--queue …] [--tries …] [-- <artisan flags>]` | Spawn ad-hoc `queue:work` workers |
| `frank worker schedule` | Spawn an ad-hoc `schedule:work` container |
| `frank worker list` | List declared + ad-hoc worker containers |
| `frank worker stop [--all]` | Stop ad-hoc workers; `--all` also stops declared ones |
| `frank worker logs [name] [-f]` | Tail logs for one or all workers |
| `frank worker top [--live] [--min-pane-width N]` | Live multi-pane CCTV view of every worker; `--live` reconciles ad-hoc churn |
| `frank watch [--status\|--stop]` | Run the code-reload watcher in the foreground, or inspect/stop the detached one |
| `frank activate` | Output eval-able shell aliases for the current project |
| `frank deactivate` | Output eval-able shell commands to remove all frank aliases |
| `frank shell-setup [--shell zsh\|bash]` | Output eval-able shell hook for auto-activation (includes completion) |
| `frank completion [bash\|zsh\|fish\|powershell]` | Output shell completion script for the given shell |
| `frank import [-f path]` | Import from a Sail `docker-compose.yml` |
| `frank eject` | Install Laravel Sail into the running containers and hand off to Sail |
| `frank version` | Print the frank binary version |

---

## Further Reading

- Dev tools — [`docs/tools.md`](docs/tools.md)
- Workers & code reload — [`docs/workers.md`](docs/workers.md)
- PHP runtimes — [`docs/runtimes.md`](docs/runtimes.md)
- Shell aliases — [`docs/shell.md`](docs/shell.md)
- Sail interop — [`docs/sail-interop.md`](docs/sail-interop.md)
- Contributing — [`docs/contributing.md`](docs/contributing.md)
