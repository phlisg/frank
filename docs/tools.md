# Dev Tools

[← Back to README](../README.md)

Frank can scaffold preconfigured dev tooling for your Laravel project. Tools are selected during `frank init` (all enabled by default) or added later with `frank tool add`. Config files are dropped once and owned by you — Frank never overwrites them.

## Available Tools

| Tool | Category | What it does |
| ---- | -------- | ------------ |
| `pint` | PHP | Laravel code style fixer. Drops `pint.json` with the Laravel preset and IDE helper excludes |
| `larastan` | PHP | Static analysis via PHPStan + Larastan. Drops `phpstan.neon` with level 5, app/ + routes/ paths |
| `rector` | PHP | Automated refactoring. Drops `rector.php` with Laravel sets, conservative levels |
| `lefthook` | Project | Git hooks manager. Assembles `lefthook.yml` with pre-commit and post-merge hooks |

## How It Works

### During `frank init`

The wizard presents a multi-select for dev tools (all pre-selected). In non-interactive mode, all tools are included unless excluded with flags:

```bash
frank init --no-rector my-app       # everything except rector
frank init --no-tools my-app        # no dev tools at all
```

Frank drops config files into the project root, patches `composer.json` with `require-dev` entries and scripts, assembles `lefthook.yml`, and runs `lefthook install` if the binary is available.

### Adding tools later

```bash
frank tool add rector
```

This appends `rector` to `frank.yaml`, drops `rector.php` if it doesn't exist, and patches `composer.json`. If `lefthook.yml` already exists, Frank prints a hint with the YAML snippet to add manually rather than overwriting your customizations.

### Cloning an existing project

When a new developer clones a repo that has `tools:` in `frank.yaml`:

```bash
git clone ...
cd my-app
frank generate    # regenerates Docker files AND reconciles dev tools
frank up -d
```

`frank generate` drops any missing config files and patches `composer.json` — idempotent and safe to run repeatedly.

## Config Files

All config files are dropped to the **project root** (not `.frank/`). They are meant to be committed to git and customized by your team.

### pint.json

```json
{
    "preset": "laravel",
    "exclude": [
        ".phpstorm.meta.php",
        "_ide_helper.php"
    ]
}
```

### phpstan.neon

```neon
includes:
    - vendor/larastan/larastan/extension.neon

parameters:
    paths:
        - app/
        - routes/
    level: 5
    excludePaths:
        - .phpstorm.meta.php
        - _ide_helper.php
```

### rector.php

Conservative defaults — all levels at 0, Laravel-aware via `LaravelSetProvider`. Uncomment `->withPhpSets()` to enable PHP-version-specific upgrades.

### lefthook.yml

Assembled based on which PHP tools are selected:

**Pre-commit** (parallel):
- **pint** — runs on staged PHP files, auto-stages fixes (`stage_fixed: true`)
- **rector** — runs on staged PHP files, auto-stages fixes
- **larastan** — runs static analysis on staged PHP files

**Post-merge** (always included):
- Auto `composer install` when `composer.lock` changes
- Auto `php artisan migrate` when migration files change
- Auto node package install when any lockfile changes (detects pnpm/bun/npm)

## Composer Integration

Frank patches `composer.json` with:

```json
{
    "require-dev": {
        "laravel/pint": "^1.0",
        "larastan/larastan": "^3.0",
        "rector/rector": "^2.0",
        "dereuromark/rector-laravel": "^2.0"
    },
    "scripts": {
        "lint": "pint --config pint.json",
        "analyse": "phpstan analyse -c phpstan.neon",
        "refactor": "rector process --config rector.php"
    }
}
```

Existing packages and scripts are never overwritten. After patching, run `frank composer install` to install the new packages (or `frank up` which auto-runs composer install).

## Running Tools

All tools run inside the container via `frank exec`:

```bash
frank exec php vendor/bin/pint
frank exec php vendor/bin/phpstan analyse -c phpstan.neon
frank exec php vendor/bin/rector process

# Or via composer scripts:
frank composer run lint
frank composer run analyse
frank composer run refactor
```

With lefthook configured, tools run automatically on `git commit` — no need to remember.

## frank.yaml

```yaml
version: 1
php:
  version: "8.5"
  runtime: frankenphp
services:
  - pgsql
  - mailpit
tools:
  - pint
  - larastan
  - rector
  - lefthook
```

The `tools:` key is a flat list. Remove an entry to stop Frank from reconciling that tool on `frank generate`. The config files themselves are yours to keep or delete.
