## 🏕️ Frank
> A config-driven Docker environment for Laravel development

Frank gives you a full Laravel development environment with zero local dependencies. No PHP, Composer, Node, or FrankenPHP installed on your machine — just Docker and the `frank` binary.

Your entire environment is described in a single `frank.yaml` file: PHP version, runtime, services, port overrides. Frank generates all the Docker files from it, so onboarding a new team member is a `git clone` and a `frank up` away.

Installing Laravel itself doesn't require a local PHP installation either — `frank install` bootstraps a fresh project inside the container, patches Vite for Docker HMR, and sets up a service-aware `.env` in one step.

Day-to-day development is smooth too. Frank's shell alias system puts `artisan`, `composer`, `php`, and your database CLI one word away — no `docker compose exec` prefixes. With auto-activation enabled, aliases appear the moment you `cd` into a project and disappear when you leave.

Already using Laravel Sail? Frank can import your existing `docker-compose.yml` and take over from there. And if you ever need to hand a project back to a Sail-based team, `frank export` has you covered.

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
frank init       # interactive wizard — creates frank.yaml and generates Docker files
frank install    # install a fresh Laravel project inside the container
frank up         # start containers, run composer install + migrate
```

Visit [http://localhost](http://localhost) once `frank up` completes.

That's it. No local PHP version juggling, no Homebrew conflicts, no "works on my machine" problems.

---

### 📖 Complete Example

A full walkthrough from zero to a running Laravel app using Docker.

**Scenario:** New project with PostgreSQL, Redis (cache + queues), and Mailpit for local email.

#### 1. Create the project directory and run the wizard

```bash
mkdir my-app && cd my-app
frank init
```

The wizard will ask:
- **PHP Version** → 8.4
- **Laravel Version** → 12.x (latest)
- **Runtime** → FrankenPHP (recommended)
- **Services** → pgsql, redis, mailpit

This writes `frank.yaml` and generates `compose.yaml`, `Dockerfile`, `Caddyfile`, `.env`, and `.env.example`.

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
| `laravel.version` | `latest` `lts` `11.*` … | `latest` | Laravel version constraint passed to Composer |
| `services` | list — see table below | `[pgsql, mailpit]` | Services to include |
| `config.<service>.port` | integer | service default | Override the host-side port mapping |

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

### 🛠 CLI Commands

| Command | Description |
| ------- | ----------- |
| `frank init` | Interactive wizard — creates `frank.yaml` and generates Docker files |
| `frank generate` | Regenerate Docker files from `frank.yaml` without prompting |
| `frank install` | Install Laravel into the project directory |
| `frank add <service>` | Add a service to `frank.yaml` and regenerate |
| `frank remove <service>` | Remove a service from `frank.yaml` and regenerate |
| `frank up [-d] [--quick] [flags…]` | Start containers; `-d` for detached, `--quick` skips composer install + migrate; all other flags pass through to `docker compose up` |
| `frank down` | Stop containers |
| `frank ps` | Show running containers |
| `frank clean` | Stop containers and delete volumes |
| `frank reset [--force]` | Delete all project files except `frank.yaml` and `.git` |
| `frank activate` | Output eval-able shell aliases for the current project |
| `frank deactivate` | Output eval-able shell commands to remove all frank aliases |
| `frank shell-setup [--shell zsh\|bash]` | Output eval-able shell hook for auto-activation (includes completion) |
| `frank completion [bash\|zsh\|fish\|powershell]` | Output shell completion script for the given shell |
| `frank import [-f path]` | Import from a Sail `docker-compose.yml` |
| `frank export [-o path]` | Export a Sail-compatible `docker-compose.yml` |
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

**Exporting back to Sail:**

```bash
frank export              # writes ./docker-compose.yml
frank export -o path/to/out.yml
```

Useful for handing a project off to a team that uses Sail, or deploying to a platform that expects Sail's compose format. Note that this is a best-effort export — custom Sail Dockerfile modifications are not preserved.

---

### ⚠️ Project Reset

Sometimes a `frank install` goes wrong halfway through, or you want to wipe the Laravel app and start fresh without touching your environment config. That's what `frank reset` is for.

```bash
frank reset
```

This stops containers, removes volumes, and deletes everything in the project directory **except** `frank.yaml`, `.git/`, `.gitignore`, `.dockerignore`, and `README.md`. Your environment definition stays intact — run `frank install` again to get a clean Laravel project with the same config.

You'll be prompted to confirm before anything is deleted. Skip the prompt with:

```bash
frank reset --force
```

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
