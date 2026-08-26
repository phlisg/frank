# Worktrees

[← Back to README](../README.md)

Frank supports running multiple instances of the same project in parallel using git worktrees. Each worktree gets its own `.frank/` directory, isolated containers, and ephemeral ports — no conflicts with the main project or other worktrees.

## Creating a Worktree

```bash
frank worktree create feature/my-branch
# ✓ Worktree created  /home/user/code/myapp-feature-my-branch
# Next steps:
#   cd /home/user/code/myapp-feature-my-branch
#   frank up
```

Worktrees are placed as siblings to your project root, named `<project>-<kebab-branch>`. Slashes and underscores in the branch name are converted to dashes.

## How It Works

When Frank detects it's running inside a git worktree:

- **Ephemeral ports**: Services use container-only port mappings instead of fixed host ports. Docker picks random available host ports, so no conflicts between worktrees.
- **Deterministic Vite port**: The Vite dev server gets a port derived from the project name (range 5174–5199), so HMR works reliably.
- **Separate project name**: Each worktree's directory name becomes its compose project name, keeping containers isolated.
- **Exempt from the auto-stop**: Frank normally runs one project at a time and `frank up` stops the previously active project (see [One Project at a Time](../README.md#one-project-at-a-time)). Worktrees are outside that rule entirely — starting one stops nothing, and it never becomes the "active" project that a later `frank up` elsewhere would stop.

- **Inherited files**: `.env`, `auth.json`, `vendor/` and `node_modules/` are copied from the main checkout at creation time. All four are gitignored, so a fresh worktree would otherwise start with none of them — see [What a Worktree Inherits](#what-a-worktree-inherits).

No config changes needed — Frank detects worktrees automatically.

Worktrees are therefore the supported way to run several Frank environments simultaneously. If you want two things up at once, make one of them a worktree.

## Managing Worktrees

### Interactive TUI

```bash
frank worktree list
```

Opens an interactive list of all linked worktrees with their container status and ports. Keybindings:

| Key | Action |
|-----|--------|
| `c` | Create a new worktree (clones the main database) |
| `C` | Create a new worktree with an empty database |
| `s` | Clone the main database into the selected worktree |
| `o` | Open in browser |
| `u` | Start containers (`frank up -d`) |
| `d` | Stop containers (`frank down`) |
| `l` | Tail container logs |
| `g` | Regenerate `.frank/` |
| `e` | Open in `$EDITOR` |
| `r` | Remove worktree — branch, containers, volumes and images (confirms first) |
| `/` | Filter by branch name |
| `q`, `ctrl+c` | Quit |

Actions run in the background and several worktrees can run at once — each is
its own compose project on ephemeral ports, so an `up` in one does not block an
`up` in another. Two actions on the *same* worktree are refused: they would
fight over one compose project. Whatever is currently running streams into the
highlighted block at the bottom of the screen.

### CLI Commands

```bash
frank worktree create <branch>              # Create a new worktree
frank worktree create <branch> --seed-db=false   # ...with an empty database
frank worktree remove <path>                # Full teardown, worktree + branch
frank worktree list                         # Interactive TUI
```

## What a Worktree Inherits

A fresh `git worktree` contains only what git tracks. Frank copies four
gitignored things across at creation so the worktree is usable immediately:

| Copied | Why |
|--------|-----|
| `.env` | Holds every project-specific key (API credentials, `MEILI_SEARCH_KEY`, `SUPER_ADMIN_EMAIL`…) and the `APP_KEY`. Generating a fresh one would mint a new `APP_KEY`, making every encrypted column in a cloned database undecryptable. Frank then patches only the keys it manages (`APP_NAME`, `APP_URL`, service hosts/ports) on top. |
| `auth.json` | Composer credentials for private packages. |
| `vendor/` | Otherwise `laravel.migrate` runs a full `composer install` on first up. |
| `node_modules/` | Otherwise the Vite sidecar runs a full `npm install` on first up. |

The two dependency trees are copied with `cp -a --reflink=auto`, so on a
copy-on-write filesystem (btrfs, XFS, APFS, ZFS) the copy is instant and costs
no extra disk. Elsewhere it falls back to a plain copy, still cheaper than
reinstalling.

`.env.example` is deliberately **not** written in a worktree. It is a committed
file owned by the branch, and rewriting it there produces a diff you never
asked for.

## Database Seeding

A worktree starts with an empty database, which is useless if you are working on
anything backed by real content — an admin panel, a CMS, a Filament resource.
Frank can clone the main project's database into it instead.

**On create.** `frank worktree create` (and `c` in the TUI) records the main
checkout's path in `.frank/.seed-db`. The first successful `frank up` reads that
marker, clones the database, then runs `php artisan migrate --force` so the
branch's own migrations land on top of the copied schema.

The main project must be **running** at that point — the dump is streamed out of
its live database container. If the clone fails, the marker stays and the next
`up` retries; a half-populated database is never left silently in place.

For a worktree that only touches logic and does not need real content, use `C`
in the TUI or `--seed-db=false` on the CLI. It starts empty and migrates from
scratch, which is faster.

**On demand.** For a worktree created before this existed, or one whose data has
gone stale, press `s` in the TUI (or use the MCP `clone-db` action). Same
mechanics, run immediately against the worktree you select.

### How the Clone Works

The dump is piped directly from one database container into the other —
`pg_dump | psql`, `mysqldump | mysql`, or `mariadb-dump | mariadb` — streamed,
so a multi-gigabyte database never lands on disk or in memory. sqlite is a plain
file copy of `database/database.sqlite`.

Dump-and-restore is used rather than copying the volume because it tolerates
version skew between the two databases and needs neither of them stopped.

MySQL and MariaDB connect as `root`: the application user cannot read the
`mysql.*` system tables that `mysqldump` touches, and lacks the privileges the
dump's session-variable statements need on restore. Both images already set
root's password to `DB_PASSWORD`. Credentials travel to the container as
environment variables (`PGPASSWORD` / `MYSQL_PWD`) so they never appear in its
process list.

### Caveats

- **Search indexes are not cloned.** Meilisearch keeps its data in its own
  volume rather than in the SQL database, so a seeded worktree starts with empty
  indexes and searches 404. Rebuild them with `php artisan scout:import
  "App\Models\YourModel"`. Frank cannot do this for you — Scout has no
  "import everything" command, and the searchable model classes are not
  knowable from `frank.yaml`.
- **A differing `APP_KEY` is warned about, not fixed.** Worktrees created before
  Frank started copying `.env` have their own key; encrypted columns in the
  cloned data will not decrypt. Copy `APP_KEY` from the main project's `.env`.
  Frank will not overwrite it, since that would invalidate anything the worktree
  already encrypted under its own key.
- **The clone is a point-in-time copy.** Nothing keeps it in sync afterwards.
  Press `s` again to refresh.

## Removing a Worktree

`frank worktree remove` (and `r` in the TUI) is a full clean, deliberately more
destructive than `frank down`:

1. `frank down` — including ad-hoc workers, which live outside `compose.yaml`
   and survive a plain `docker compose down`.
2. `docker compose down -v --remove-orphans --rmi local` — containers, named
   volumes, orphans and locally-built images.
3. Removes the app image tags directly, which `--rmi local` skips because the
   workers and the Vite sidecar name their image explicitly.
4. `git worktree remove --force` and `git branch -D`.

Everything removed is scoped to the worktree's own compose project, so once its
directory and branch are gone nothing can address any of it again — it is pure
wasted disk. **The database volume goes with it.** Since a seeded worktree holds
a full copy of the main project's data, that can be a lot of disk; it also means
anything created only in that worktree is gone for good. The TUI names what will
be deleted before asking for confirmation.

## MCP Integration

The `frank_worktrees` MCP tool exposes worktree management to AI assistants. See [`docs/mcp.md`](mcp.md) for details.
