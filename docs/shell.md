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

**Auto-activation** — the recommended setup for day-to-day use:

```bash
echo 'eval "$(frank config shell setup)"' >> ~/.zshrc   # or ~/.bashrc
```

This installs a `chpwd` hook (zsh) or `cd` wrapper (bash) that watches for `frank.yaml` as you navigate directories. Step into a Frank project and aliases activate automatically. Step out and they're gone. No manual `eval` needed, no aliases leaking between projects.

Shell completion is wired up at the same time — `frank <tab>`, `frank new <tab>`, and subcommand completion all work out of the box once `shell-setup` is in your profile. If you want completion without the auto-activation hook, you can add it separately:

```bash
echo 'eval "$(frank config shell completion zsh)"' >> ~/.zshrc   # or bash / fish / powershell
```
