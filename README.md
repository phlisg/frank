## 🏕️ Frank
> A config-driven Docker environment for Laravel development

Frank gives you a full Laravel development environment with zero local dependencies. No PHP, Composer, Node, or FrankenPHP installed on your machine — just Docker and the `frank` binary.

Your entire environment is described in a single `frank.yaml` file: PHP version, runtime, services, port overrides. Frank generates all the Docker files from it, so onboarding a new team member is a `git clone` and a `frank up` away.

Installing Laravel itself doesn't require a local PHP installation either — `frank install` bootstraps a fresh project inside the container, patches Vite for Docker HMR, and sets up a service-aware `.env` in one step.

Day-to-day development is smooth too. Frank's shell alias system puts `artisan`, `composer`, `php`, and your database CLI one word away — no `docker compose exec` prefixes. With auto-activation enabled, aliases appear the moment you `cd` into a project and disappear when you leave.

Already using Laravel Sail? Frank can import your existing `docker-compose.yml` and take over from there. And if you ever need to hand a project back to a Sail-based team, `frank eject` has you covered.

Frank is distributed as a single static binary. No Node, no Python, no package managers — just download and drop it in your `PATH`.

---

### ⚡ Install

The preferred way to install Frank is via `go install`:

```bash
go install github.com/phlisg/frank@latest
```

**Linux** — alternatively, download a pre-built binary from [GitHub Releases](https://github.com/phlisg/frank/releases):

```bash
curl -Lo frank https://github.com/phlisg/frank/releases/latest/download/frank-linux-amd64
chmod +x frank
sudo mv frank /usr/local/bin/
```

**macOS** — `go install` is the only supported method. Pre-built binaries are unsigned and macOS Gatekeeper will quarantine them.

> **Tip:** For better Docker volume mount performance, enable VirtioFS in Docker Desktop → Settings → General → "Use VirtioFS for file sharing".

**WSL (Windows)** — the Linux binary works as-is; alternatively use `go install` as above.

```bash
curl -Lo frank https://github.com/phlisg/frank/releases/latest/download/frank-linux-amd64
chmod +x frank
sudo mv frank /usr/local/bin/
```

> **Note:** Make sure Docker Desktop has the **WSL 2 backend** enabled (Settings → Resources → WSL Integration).

---

### 🚀 Quick Start

```bash
frank init my-app   # interactive wizard — creates my-app/, writes frank.yaml, generates Docker files
cd my-app
frank install       # install a fresh Laravel project inside the container
frank up            # start containers, run composer install + migrate
```

Or skip every prompt with inline flags:

```bash
frank init --php 8.5 --laravel 12 --runtime frankenphp --with="pgsql,mailpit" my-app
```

Visit [http://localhost](http://localhost) once `frank up` completes.

That's it. No local PHP version juggling, no Homebrew conflicts, no "works on my machine" problems.

---

### 📖 Complete Example

A full walkthrough from zero to a running Laravel app using Docker.

**Scenario:** New project with PostgreSQL, Redis (cache + queues), and Mailpit for local email.

#### 1. Create the project directory and run the wizard

```bash
frank init my-app
cd my-app
```

The wizard will ask:
- **PHP Version** → 8.4
- **Laravel Version** → 12.x (latest)
- **Runtime** → FrankenPHP (recommended)
- **Services** → pgsql, redis, mailpit

Prefer flags? Skip every prompt in one shot:

```bash
frank init --php 8.4 --laravel 12 --runtime frankenphp --with="pgsql,redis,mailpit" my-app
```

Either way, Frank writes `frank.yaml` and generates `compose.yaml`, `Dockerfile`, `Caddyfile`, `.env`, and `.env.example`.

#### 2. Install Laravel

```bash
frank install
```

Spins up a disposable `composer:latest` container, creates a fresh Laravel project, moves the files into your directory, and patches Vite for Docker HMR. No local PHP required.

#### 3. Start containers

```bash
frank up -d
```

Starts all services in the background, runs `composer install`, and runs `php artisan migrate`.

Visit [http://localhost](http://localhost) — you should see the Laravel welcome page.

#### 4. Set up shell aliases (once)

```bash
eval "$(frank shell-setup)" >> ~/.zshrc   # or ~/.bashrc
source ~/.zshrc
```

From now on, aliases activate automatically when you `cd` into a Frank project. You now have:

```bash
artisan make:model Post -mcr    # runs inside the container
composer require spatie/laravel-query-builder
php -r "echo PHP_VERSION;"
tinker                          # interactive REPL with full app context
psql                            # drop into PostgreSQL
```

#### 5. Configure `.env` for Redis cache and queues

The generated `.env` already has `REDIS_HOST=redis`. Just set the drivers:

```dotenv
CACHE_STORE=redis
QUEUE_CONNECTION=redis
```

Then restart if needed:

```bash
frank down && frank up -d
```

#### 6. Day-to-day development

```bash
artisan make:controller Api/PostController --resource
artisan migrate:fresh --seed
artisan queue:work                  # runs in foreground inside container
npm run dev                         # Vite HMR on http://localhost:5173
```

Visit [http://localhost:8025](http://localhost:8025) for the Mailpit inbox — any mail your app sends in local dev lands here.

#### 7. Add a service later

```bash
frank add meilisearch
```

Updates `frank.yaml`, regenerates `compose.yaml` and `.env.example`. Run `frank up -d` again to start the new container.

#### 8. Onboard a team member

They only need:

```bash
git clone ...
cd my-app
frank up -d        # containers start, migrate runs
```

That's it — no local PHP, no Composer, no version conflicts.

---

### 🗒 Example frank.yaml

Not sure where to start? Here's a solid default you can drop straight into your project — Laravel 12 LTS, PHP-FPM, MariaDB, Memcached, and Mailpit for local mail:

```yaml
version: 1

php:
  version: "8.4"
  runtime: "fpm"

laravel:
  version: "12.*"

services:
  - mariadb
  - memcached
  - mailpit
```

Save this as `frank.yaml` in your project root, then run `frank generate` to create all the Docker files. Adjust the PHP version, runtime, or services to taste — see the full reference below.

---

### 📋 Requirements

| Tool | Purpose |
| ---- | ------- |
| [Docker](https://docs.docker.com/get-docker/) | Container runtime |

Everything else — PHP, Composer, Node, FrankenPHP — runs inside containers.

---

### 📄 frank.yaml

`frank.yaml` is the single source of truth for your environment. All Docker files (`compose.yaml`, `Dockerfile`, `.env`, etc.) are generated from it. You commit `frank.yaml` to git; the generated files can be gitignored or committed alongside it — your choice.

**Default (generated by `frank init`):**

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

**All options:**

| Key | Values | Default | Description |
| --- | ------ | ------- | ----------- |
| `php.version` | `8.2` `8.3` `8.4` `8.5` | `8.5` | PHP version |
| `php.runtime` | `frankenphp` `fpm` | `frankenphp` | Web server runtime |
| `laravel.version` | `latest` `lts` `12.*` `13.*` … | `latest` | Laravel version constraint passed to Composer |
| `services` | list — see table below | `[pgsql, mailpit]` | Services to include |
| `config.<service>.port` | integer | service default | Override the host-side port mapping |
| `workers.schedule` | boolean | `false` | Run `php artisan schedule:work` in a dedicated container |
| `workers.queue` | list — see Workers section | `[]` | Declare long-running `queue:work` worker pools |

After editing `frank.yaml`, run `frank generate` to regenerate Docker files, or simply `frank up` — it auto-regenerates if `frank.yaml` is newer than `compose.yaml`.

---

### 🗂 Supported Services

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

### ⚙️ PHP Runtimes

Frank supports two runtimes. The right choice depends on how close you want your dev environment to match production.

**`frankenphp` (default) — recommended for most projects**

FrankenPHP is a modern PHP app server built on top of Caddy. It runs your Laravel app in a single container with HTTP/2 and HTTPS out of the box. It's fast to start, simple to configure, and great for greenfield projects or teams that don't have a strong opinion about their production stack.

Choose `frankenphp` if: you're starting a new project, you deploy to a platform like Laravel Cloud or Fly.io, or you just want things to work with minimal fuss.

**`fpm` — for production parity with traditional stacks**

PHP-FPM pairs with a separate Nginx container, matching the classic shared-hosting and VPS setup. More moving parts, but familiar if your production server runs Nginx + PHP-FPM.

Choose `fpm` if: your production environment uses Nginx + PHP-FPM, or you're maintaining an existing project that was built with that stack in mind.

| Runtime | Containers | Best for |
| ------- | ---------- | -------- |
| `frankenphp` | Single (app) | New projects, modern deployments |
| `fpm` | Two (app + nginx) | Production parity with Nginx stacks |

---

### 👷 Workers

Frank can run Laravel's scheduler and queue workers as dedicated long-running containers alongside `laravel.test`. Both are opt-in and declared in `frank.yaml`.

**`workers.schedule`** — when `true`, Frank generates a `laravel.schedule` container running `php artisan schedule:work`. Replaces the traditional cron entry; stays alive across `frank up`/`frank down` cycles.

**`workers.queue`** — a list of worker *pools*. Each pool maps one or more queues to a fixed number of `queue:work` containers. Pools are useful when you want to isolate workload — e.g. one pool chewing on slow image-processing jobs, another draining a fast `notifications` queue.

```yaml
workers:
  schedule: true
  queue:
    - name: default       # optional; defaults to queues[0]
      queues: [default]
      count: 2
    - name: media
      queues: [media, thumbnails]
      count: 1
      tries: 3            # optional
      timeout: 120        # optional
      memory: 512         # optional
      sleep: 3            # optional
      backoff: 5          # optional
```

Omitting `queues` defaults to `[default]`; omitting `name` derives it from `queues[0]`. Pool names must be unique and match `[a-z0-9_-]+`.

Declared workers are ordinary compose services — start with `frank up`, stop with `frank down`, tail with `frank worker logs`.

#### Bootstrapping via `frank init`

The interactive wizard now includes a "Schedule worker" yes/no prompt and a "Queue workers" 0–4 prompt. Answer them and Frank writes the corresponding `workers:` block into `frank.yaml`. Prefer flags? Skip those prompts:

```bash
frank init --php 8.4 --laravel 12 --runtime frankenphp \
  --with="pgsql,redis,mailpit" --schedule --queue-count 2 my-app
```

That produces a `frank.yaml` with `workers.schedule: true` and a single `default` pool of 2 queue workers. For more exotic pool layouts, edit `frank.yaml` directly and run `frank generate`.

#### Ad-hoc workers

Sometimes you just want to fire up a worker one-off — debugging a job, draining a backlog, etc. Frank supports this without touching `frank.yaml`:

```bash
frank worker queue                           # one ad-hoc queue:work on "default"
frank worker queue --count 3 --queue media   # three workers on "media"
frank worker queue --tries 3 --timeout 120   # tune per invocation
frank worker queue -- --once                 # pass extra artisan flags after `--`
frank worker schedule                        # ad-hoc schedule:work
frank worker list                            # show declared + ad-hoc workers
frank worker logs                            # tail all workers
frank worker logs laravel.queue.default.1    # tail a single worker
frank worker stop                            # stop ad-hoc workers
frank worker stop --all                      # stop declared workers too
```

Ad-hoc workers are labelled `frank.worker=adhoc` so `frank down` cleans them up automatically — no orphans.

#### Code reload: `frank watch`

Queue workers bootstrap your Laravel app once and hold it in memory — great for throughput, painful for development. Edit a class, and without a reload the worker keeps running the old code.

`frank watch` solves this. It's a host-side file watcher (uses `fsnotify`) that observes `app/`, `bootstrap/`, `config/`, `database/`, `lang/`, `resources/views/`, `routes/`, `.env`, and `composer.lock`. On change it runs `php artisan queue:restart` and, if `workers.schedule` is enabled, restarts the schedule container. Debounced, so a rapid save flurry fires once.

```bash
frank watch               # foreground, Ctrl-C to stop
frank watch --status      # show the detached watcher's pid, uptime, state
frank watch --stop        # SIGTERM the detached watcher
```

You rarely need to invoke it directly: **`frank up` auto-spawns the watcher** when `workers.schedule` or any `workers.queue` pool is declared, and **`frank down` stops it**. Foreground `frank up` runs the watcher in-process; `frank up -d` spawns a detached one that writes to `.frank/watch.log`. Use `frank watch --status` to inspect the detached one.

---

### 🛠 CLI Commands

| Command | Description |
| ------- | ----------- |
| `frank init [dir]` | Interactive wizard — creates `frank.yaml` and generates Docker files; if `dir` is given, creates and targets that directory. Flags `--php`, `--laravel`, `--runtime`, `--with`, `--schedule`, `--queue-count` skip the corresponding prompts for non-interactive use |
| `frank generate` | Regenerate Docker files from `frank.yaml` without prompting |
| `frank install` | Install Laravel into the project directory |
| `frank add <service>` | Add a service to `frank.yaml` and regenerate |
| `frank remove <service>` | Remove a service from `frank.yaml` and regenerate |
| `frank up [-d] [--quick] [flags…]` | Start containers; `-d` for detached, `--quick` skips composer install + migrate; all other flags pass through to `docker compose up`. Auto-spawns the watcher when workers are declared |
| `frank down` | Stop containers and the watcher |
| `frank ps` | Show running containers |
| `frank clean` | Stop containers and delete volumes |
| `frank worker queue [--count N] [--queue …] [--tries …] [-- <artisan flags>]` | Spawn ad-hoc `queue:work` workers |
| `frank worker schedule` | Spawn an ad-hoc `schedule:work` container |
| `frank worker list` | List declared + ad-hoc worker containers |
| `frank worker stop [--all]` | Stop ad-hoc workers; `--all` also stops declared ones |
| `frank worker logs [name] [-f]` | Tail logs for one or all workers |
| `frank watch [--status\|--stop]` | Run the code-reload watcher in the foreground, or inspect/stop the detached one |
| `frank activate` | Output eval-able shell aliases for the current project |
| `frank deactivate` | Output eval-able shell commands to remove all frank aliases |
| `frank shell-setup [--shell zsh\|bash]` | Output eval-able shell hook for auto-activation (includes completion) |
| `frank completion [bash\|zsh\|fish\|powershell]` | Output shell completion script for the given shell |
| `frank import [-f path]` | Import from a Sail `docker-compose.yml` |
| `frank eject` | Install Laravel Sail into the running containers and hand off to Sail |
| `frank version` | Print the frank binary version |

---

### 🐚 Shell Aliases

Running `docker compose exec app php artisan` every time gets old fast. Frank's shell alias system solves this by injecting short, familiar commands directly into your shell session — so you type `artisan migrate` and Frank routes it into the right container automatically.

It's modelled after Python's virtualenv activation: explicit, per-project, and easy to turn off.

**Manual activation** — run once per terminal session:

```bash
eval "$(frank activate)"
```

Your prompt gains a `(frank)` prefix so you always know aliases are active. Run `deactivate` to remove them and restore your original prompt.

**What gets activated:**

| Alias | Runs |
| ----- | ---- |
| `composer` | `docker compose exec app composer` |
| `artisan` | `docker compose exec app php artisan` |
| `php` | `docker compose exec app php` |
| `tinker` | `docker compose exec app php artisan tinker` |
| `npm` | `docker compose exec app npm` |
| `bun` | `docker compose exec app bun` |
| `psql` | `docker compose exec db psql …` *(pgsql only)* |
| `mysql` | `docker compose exec db mysql …` *(mysql/mariadb only)* |
| `redis-cli` | `docker compose exec redis redis-cli` *(redis only)* |

The database aliases are only added when the matching service is configured, so `psql` won't appear in a MySQL project and vice versa.

**Auto-activation** — the recommended setup for day-to-day use:

```bash
eval "$(frank shell-setup)"   # add once to ~/.zshrc or ~/.bashrc
```

This installs a `chpwd` hook (zsh) or `cd` wrapper (bash) that watches for `frank.yaml` as you navigate directories. Step into a Frank project and aliases activate automatically. Step out and they're gone. No manual `eval` needed, no aliases leaking between projects.

Shell completion is wired up at the same time — `frank <tab>`, `frank init --dir <tab>`, and subcommand completion all work out of the box once `shell-setup` is in your profile. If you want completion without the auto-activation hook, you can add it separately:

```bash
eval "$(frank completion zsh)"   # or bash / fish / powershell
```

---

### ⛵ Sail Interop

Frank can read and write Laravel Sail's `docker-compose.yml` format, making it easy to migrate an existing Sail project to Frank or hand a Frank project off to someone who prefers Sail.

**Migrating from Sail:**

```bash
frank import              # reads ./docker-compose.yml
frank import -f path/to/docker-compose.yml
```

Frank inspects the Sail compose file, detects your PHP version and services, writes `frank.yaml`, and regenerates all Docker files. Your existing Sail compose file is not modified.

**Ejecting to Sail:**

```bash
frank eject
```

Installs Laravel Sail into the running containers (`composer require laravel/sail` + `sail:install`) using the services from your `frank.yaml`. Useful for handing a project off to a team that prefers Sail. Requires containers to be running — run `frank up` first, then run `./vendor/bin/sail up` to continue with Sail.

---

### 🧑‍💻 Developer Guide

#### Running locally

```bash
go build -o frank .
./frank
```

For live reload during development:

```bash
go tool air
```

#### Running tests

```bash
go test ./...
```

#### Releasing a new version

Frank uses tag-based releases. Pushing a tag triggers the GitHub Actions workflow, which builds binaries for all platforms and creates a GitHub release.

```bash
git tag v1.2.3
git push origin v1.2.3
```

The version is injected into the binary at build time — `frank version` will return the tag name.
