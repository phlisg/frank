# Testing

[← Back to README](../README.md)

Frank automatically sets up a dedicated testing database so `RefreshDatabase` never touches your dev data. Works out of the box for PostgreSQL, MySQL, and MariaDB — no configuration needed.

## How it works

When you run `frank generate` (or `frank new` / `frank setup`), Frank:

1. **Writes an init script** to `.frank/scripts/` — a short SQL or shell script that creates a `testing` database inside the same DB container.
2. **Mounts the script** into the container's `docker-entrypoint-initdb.d/` so it runs automatically on first boot.
3. **Patches `phpunit.xml`** — sets `DB_CONNECTION` and `DB_DATABASE` so Laravel's test runner uses the testing database.

SQLite projects skip all of this — Laravel's default `:memory:` works fine.

## Running tests

```bash
frank test                            # run all tests
frank test -- --parallel              # Pest parallel
frank test -- --filter=UserTest       # filter by name
frank test -- --parallel --processes=4
```

`frank test` executes `php artisan test` inside the `laravel.test` container. Artisan and Pest flags go after `--`, following the same passthrough convention as other Frank commands.

Containers must be running — if they're not, Frank will tell you to `frank up` first.

## Pest parallel support

The init scripts grant the DB user privileges on `testing%` (wildcard). This covers the `testing_1`, `testing_2`, … databases that Pest creates when running with `--parallel`. No extra setup required.

## Volume gotcha

Database init scripts only execute when the data volume is empty (first container start). If you added Frank to an existing project — or upgraded from a version without testing database support — you need to recreate the volume:

```bash
frank down -v    # remove volumes
frank up         # recreate with init script
```

This is a one-time operation. After the init script runs, the testing database persists across restarts.

## Ejecting to Sail

`frank eject` restores `phpunit.xml` to Laravel's defaults (`DB_CONNECTION=sqlite`, `DB_DATABASE=:memory:`). Sail has its own init script mechanism and will set up testing databases via `vendor/laravel/sail/database/`.

## Init scripts by engine

| Engine | Script | Mechanism |
| ------ | ------ | --------- |
| PostgreSQL | `create-testing-database.sql` | `\gexec` — idempotent `CREATE DATABASE` |
| MySQL | `create-testing-database.sh` | `mysql` CLI + wildcard `GRANT` on `testing%` |
| MariaDB | `create-testing-database.sh` | `/usr/bin/mariadb` CLI + wildcard `GRANT` on `testing%` |
| SQLite | — | Not needed (`:memory:` works natively) |
