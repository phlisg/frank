# Workers

[← Back to README](../README.md)

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

## Bootstrapping via `frank new`

The interactive wizard now includes a "Schedule worker" yes/no prompt and a "Queue workers" 0–4 prompt. Answer them and Frank writes the corresponding `workers:` block into `frank.yaml`. Prefer flags? Skip those prompts:

```bash
frank new --php 8.4 --laravel 12 --runtime frankenphp \
  --with="pgsql,redis,mailpit" --schedule --queue-count 2 my-app
```

That produces a `frank.yaml` with `workers.schedule: true` and a single `default` pool of 2 queue workers. For more exotic pool layouts, edit `frank.yaml` directly and run `frank generate`.

## Ad-hoc workers

Sometimes you just want to fire up a worker one-off — debugging a job, draining a backlog, etc. Frank supports this without touching `frank.yaml`:

```bash
frank worker queue                           # one ad-hoc queue:work on "default"
frank worker queue --count 3 --queue media   # three workers on "media"
frank worker queue --tries 3 --timeout 120   # tune per invocation
frank worker queue -- --once                 # pass extra artisan flags after `--`
frank worker schedule                        # ad-hoc schedule:work
frank worker ps                              # show declared + ad-hoc workers
frank worker logs                            # tail all workers
frank worker logs laravel.queue.default.1    # tail a single worker
frank worker stop                            # stop ad-hoc workers
frank worker stop --all                      # stop declared workers too
```

Ad-hoc workers are labelled `frank.worker=adhoc` so `frank down` cleans them up automatically — no orphans.

## Live multi-pane view: `frank worker top`

`frank worker top` opens a CCTV-style terminal UI that tails every worker's
log in its own pane. One full-width row for the scheduler, one row per
declared queue pool, and a trailing row for any ad-hoc workers. Memory
usage sits in each pane's title bar.

```bash
frank worker top                       # snapshot layout — ad-hoc changes ignored
frank worker top --live                # poll for new/removed ad-hoc workers every 2s
frank worker top --min-pane-width 40   # force denser panes on ultrawide terminals
```

**Key bindings:**

| Key                 | Action                                 |
| ------------------- | -------------------------------------- |
| `q`, `Ctrl-C`       | Quit (workers keep running)            |
| `Tab`, `←`, `→`     | Cycle focus between panes              |
| `Enter`             | Zoom focused pane full-screen          |
| Left-click pane     | Focus + zoom that pane in one shot     |
| `Esc`               | Return from zoom to grid (click zoomed pane also unzooms) |
| `PgUp`, `PgDn`      | Scroll back through logs (zoom only)   |
| `g`, `G`            | Jump to top / bottom of scrollback     |

The TUI is read-only: it never starts, stops, or restarts a container.
If no workers are running when you launch, it exits with a hint pointing
you at `frank.yaml` or `frank worker queue`.

Border colors reflect state: green (running), yellow (no stats sample in
the last 10 s — likely stalled), red (`[exited N]` — the container exited
with code `N`), thick bright-magenta (currently focused pane). Worker log
output arrives with ANSI color intact; Laravel's green `DONE` and red
`FAIL` are visible as written.

On launch each pane seeds with the last 25 log lines (`docker logs --tail 25`)
and then follows live — avoids a thousand-job replay flooding the grid when
you've been processing a backlog.

Design details: [`docs/superpowers/specs/2026-04-19-worker-top-tui-design.md`](superpowers/specs/2026-04-19-worker-top-tui-design.md).

## Code reload: `frank watch`

Queue workers bootstrap your Laravel app once and hold it in memory — great for throughput, painful for development. Edit a class, and without a reload the worker keeps running the old code.

`frank watch` solves this. It's a host-side file watcher (uses `fsnotify`) that observes `app/`, `bootstrap/`, `config/`, `database/`, `lang/`, `resources/views/`, `routes/`, `.env`, and `composer.lock`. On change it runs `php artisan queue:restart` and, if `workers.schedule` is enabled, restarts the schedule container. Debounced, so a rapid save flurry fires once.

```bash
frank watch               # foreground, Ctrl-C to stop
frank watch --status      # show the detached watcher's pid, uptime, state
frank watch --stop        # SIGTERM the detached watcher
```

You rarely need to invoke it directly: **`frank up` auto-spawns the watcher** when `workers.schedule` or any `workers.queue` pool is declared, and **`frank down` stops it**. Foreground `frank up` runs the watcher in-process; `frank up -d` spawns a detached one that writes to `.frank/watch.log`. Use `frank watch --status` to inspect the detached one.
