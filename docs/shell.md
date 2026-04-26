# Shell Aliases

[← Back to README](../README.md)

Running `docker compose exec app php artisan` every time gets old fast. Frank's shell alias system solves this by injecting short, familiar commands directly into your shell session — so you type `artisan migrate` and Frank routes it into the right container automatically.

It's modelled after Python's virtualenv activation: explicit, per-project, and easy to turn off.

**Manual activation** — run once per terminal session:

```bash
eval "$(frank config shell activate)"
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

## Custom Aliases

You can define your own aliases in `frank.yaml` under the `aliases` key. Two forms are supported:

**String form** — runs inside the container (most common):

```yaml
aliases:
  fresh: "php artisan migrate:fresh --seed"
  seed: "php artisan db:seed"
  logr: "truncate -s 0 storage/logs/laravel.log"
  stan: "php vendor/bin/phpstan analyse"
```

These are automatically prefixed with `docker compose exec --user sail laravel.test`, so `fresh` becomes:

```bash
docker compose --project-directory . -f .frank/compose.yaml exec --user sail laravel.test php artisan migrate:fresh --seed
```

**Map form** — runs on the host (for commands that don't belong in a container):

```yaml
aliases:
  open:
    cmd: "open http://localhost"
    host: true
  logs:
    cmd: "frank compose logs -f laravel.test"
    host: true
```

Host aliases run the command directly on your machine, no Docker wrapping.

**Mixing both forms:**

```yaml
aliases:
  # Container commands (string shorthand)
  fresh: "php artisan migrate:fresh --seed"
  seed: "php artisan db:seed"
  logr: "truncate -s 0 storage/logs/laravel.log"

  # Host commands (map form)
  open:
    cmd: "open http://localhost"
    host: true
```

**Rules:**

- Alias names must be valid shell identifiers: letters, numbers, underscores, hyphens (must start with a letter or underscore)
- Names are checked case-insensitively — `Fresh` collides with `fresh`
- Can't shadow built-in aliases (`artisan`, `composer`, `php`, `npm`, etc.)
- Aliases that shadow common shell builtins (`cd`, `ls`, `echo`) produce a warning but are allowed

Custom aliases activate alongside built-ins when you run `frank config shell activate` or when auto-activation triggers via `frank config shell setup`.

**After editing aliases in frank.yaml**, reload them by either:

- `cd .` (if auto-activation is set up — the chpwd hook re-reads `frank.yaml`)
- `eval "$(frank config shell activate)"` (manual reload)

**Auto-activation** — the recommended setup for day-to-day use:

```bash
echo 'eval "$(frank config shell setup)"' >> ~/.zshrc   # or ~/.bashrc
```

This installs a `chpwd` hook (zsh) or `cd` wrapper (bash) that watches for `frank.yaml` as you navigate directories. Step into a Frank project and aliases activate automatically. Step out and they're gone. No manual `eval` needed, no aliases leaking between projects.

Shell completion is wired up at the same time — `frank <tab>`, `frank new <tab>`, and subcommand completion all work out of the box once `shell-setup` is in your profile. If you want completion without the auto-activation hook, you can add it separately:

```bash
echo 'eval "$(frank config shell completion zsh)"' >> ~/.zshrc   # or bash / fish / powershell
```
