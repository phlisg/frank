# Configuration Commands

[← Back to README](../README.md)

Frank's `config` namespace groups all configuration management. View, edit, and modify `frank.yaml` without opening it manually — and manage shell integration from one place.

## frank config show

Print the fully resolved configuration to stdout. Defaults are filled in, validation is applied — this is what Frank actually uses.

```bash
$ frank config show
version: 1
php:
  version: "8.5"
  runtime: frankenphp
laravel:
  version: latest
services:
  - pgsql
  - mailpit
workers:
  schedule: true
  queue:
    - name: default
      queues:
        - default
      count: 1
node:
  packagemanager: npm
```

Useful for debugging ("what does Frank think my config is?") and for piping into other tools.

## frank config edit

Opens `frank.yaml` in your editor. Checks `$EDITOR`, then `$VISUAL`, then falls back to `vi`.

```bash
frank config edit
```

No rebuild is triggered — you may not have saved, or you may want to review changes first. Run `frank generate` or `frank up` afterwards to apply changes.

## frank config set

Modify a scalar value in `frank.yaml` from the command line. Preserves comments and formatting — Frank edits the YAML node tree directly, not a struct round-trip.

```bash
frank config set php.version 8.4
frank config set php.runtime fpm
frank config set laravel.version "13.*"
frank config set node.packageManager pnpm
```

After setting, Frank automatically regenerates `.frank/` files and prompts to rebuild containers if they're running.

**Supported keys:**

| Key | Valid values | Default |
| --- | ----------- | ------- |
| `php.version` | `8.2`, `8.3`, `8.4`, `8.5` | `8.5` |
| `php.runtime` | `frankenphp`, `fpm` | `frankenphp` |
| `laravel.version` | `12.*`, `13.*`, `latest` | `latest` |
| `node.packageManager` | `npm`, `pnpm`, `bun` | `npm` |
| `dev.enabled` | `true`, `false` | `true` |
| `dev.command` | any shell command | derived from `node.packageManager` |

Unknown keys or invalid values produce an error listing valid options. Shell completion is available for both keys and values.

### Dev server (`dev`)

Frank runs the frontend dev server (Vite) as a managed compose sidecar
(`laravel.vite`), started by `frank up` and stopped by `frank down`. Attach to
its output with `frank dev` (Ctrl-C detaches; the server keeps running).

```yaml
dev:
  enabled: true          # false omits the laravel.vite service entirely
  command: ""            # empty → derived from node.packageManager
```

When `command` is empty, Frank derives it from the package manager — e.g. for
`pnpm` it runs `pnpm install` (only when `node_modules` is absent) then
`pnpm dev`. Set `command` to override verbatim (it is run via `sh -c` inside the
container) — useful for extra flags, a different script, or skipping the install:

```yaml
dev:
  command: "npm run dev -- --force"
```

Keep Vite listening on **port 5173 inside the container** — that's the only port
compose publishes (`<host>:5173`). Telling Vite to use a different port in
`command` leaves it unmapped and unreachable from the host. Change the *host*
port via worktree mode, not here.

With `dev.enabled: false`, no dev-server container is created and the Vite port
is left unmapped.

For services, workers, tools, and aliases, use the dedicated commands instead:

- `frank add <service>` / `frank remove <service>`
- `frank worker queue` / `frank worker schedule`
- `frank tool add <tool>`
- Edit aliases directly in `frank.yaml` (see [`docs/shell.md`](shell.md#custom-aliases))

## Extra services (`extra_services`)

Frank's service catalogue (`pgsql`, `redis`, `mailpit`, …) is closed — each entry
needs templates shipped in a Frank release. `extra_services` is the escape hatch:
raw Docker Compose service blocks, written in `frank.yaml` and merged verbatim
into `.frank/compose.yaml`.

```yaml
services:
  - pgsql
  - mailpit

extra_services:
  gotenberg:
    image: gotenberg/gotenberg:8
    ports: ["3000:3000"]
    dot_env:
      GOTENBERG_URL: http://gotenberg:3000
```

Anything Compose understands works — `command`, `volumes`, `healthcheck`,
`depends_on`, `deploy`, keys that ship after this was written. Frank does not
model the block; it copies it.

### `environment:` vs `dot_env:`

These sit two lines apart and point in opposite directions:

| Key | Owner | Destination |
| --- | ----- | ----------- |
| `environment:` | Compose | Variables **inside the custom container** |
| `dot_env:` | Frank | Lines written into the **Laravel app's `.env` on the host** |

Gotenberg does not read `GOTENBERG_URL`. Laravel does. `dot_env` is the inline
replacement for the `env.tmpl` that built-in services have; Frank strips it out
of the block before writing `compose.yaml` and never passes it to Compose.

`dot_env` values are container-network addresses (`http://gotenberg:3000`) and
are identical in every worktree — ephemeral host ports affect only what the
*host* connects to, which is read from `frank compose ps`, never from `.env`.

### What Frank validates

Everything else in the block passes through untouched — an unrecognised key is
the feature working as intended, so no unknown-key warning is emitted (unlike
`dev` or `workers`).

- The service name matches `[a-z0-9][a-z0-9_.-]*`.
- The name does not collide with a built-in service or one Frank generates:
  `laravel.test`, `laravel.vite`, `nginx`, `migrate`, `schedule`, and
  `queue.<pool>.<n>` for each declared pool.
- The block sets `image` or `build` — Compose rejects a service with neither,
  but far from `frank.yaml`.
- `dot_env` keys match `[A-Z][A-Z0-9_]*` and values are scalars (string, number,
  boolean).
- A named volume does not collide with one Frank owns (`pgsql_data`,
  `mysql_data`, `mariadb_data`, `redis_data`, `meilisearch_data`).

A malformed block that survives this fails at `docker compose up` with Compose's
own error message, which is better than anything Frank would write.

### What Frank does to the block

- **`networks`** defaults to `[frank]` when absent. An explicit `networks:` is a
  deliberate act and is left alone.
- **Ports are rewritten in worktrees.** In worktree mode every published host
  port is dropped, so `"3000:3000"`, `"8080:3000"` and `"127.0.0.1:8080:3000"`
  all become `"3000"` (a `/tcp` or `/udp` suffix is preserved), and a long-form
  entry loses its `published`. Without this the second worktree of a repo fails
  to start on a host-port conflict. Outside worktrees the published port is kept
  and checked for collisions against every other service.
- **Named volumes are auto-declared.** Any volume source that is not a host path
  (`.`, `/`, `~`, `$`) gets a top-level `{driver: local}` entry — Compose errors
  on an undeclared named volume, and `frank.yaml` has nowhere else to declare it.
- **No `depends_on` is injected in either direction.** Extra services are
  uncritical by definition: the app does not wait for them and they do not wait
  for the database. A service that needs ordering writes its own `depends_on` —
  passthrough allows it.

### Caveat: `.env.example` is not redacted for custom keys

Frank redacts a fixed key set (`APP_KEY`, `DB_PASSWORD`, `DB_URL`,
`REDIS_PASSWORD`) when writing `.env.example`. Custom `dot_env` keys are not in
that set, so a secret placed in a `dot_env` value is written **verbatim** into
`.env.example` — a committed file. Keep credentials out of `dot_env`: put the
public address there and read the secret from `.env` directly.

### Interacting with an extra service

Frank grows no commands for extra services — the existing passthrough already
covers the whole surface:

| Need | Command |
|---|---|
| Shell / one-off command in the service | `frank compose exec gotenberg sh` |
| Follow its logs | `frank compose logs -f gotenberg` |
| Restart it alone | `frank compose restart gotenberg` |
| Status and published host port | `frank compose ps` |
| Reach it from the app container | `frank exec curl http://gotenberg:3000/health` |

`frank exec` is hardwired to `laravel.test` and always will be — it is the app
shell. Anything aimed at another container goes through `frank compose exec`,
which is a raw passthrough (flags are not parsed, so `frank compose exec -T …`
works unchanged). Both honour Frank's `--project-directory . -f .frank/compose.yaml`
invariant, so they address the right project from anywhere in the tree.

## frank config shell

Shell integration subcommands — aliases, hooks, and completion. Full documentation: [`docs/shell.md`](shell.md).

| Command | Description |
| ------- | ----------- |
| `frank config shell activate` | Output eval-able aliases for the current project |
| `frank config shell deactivate` | Remove all frank-managed aliases |
| `frank config shell setup [--shell zsh\|bash]` | Output auto-activation hook for your shell profile |
| `frank config shell completion [bash\|zsh\|fish\|powershell]` | Output shell completion script |

Quick setup:

```bash
echo 'eval "$(frank config shell setup)"' >> ~/.zshrc   # or ~/.bashrc
```
