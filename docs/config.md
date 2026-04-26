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

Unknown keys or invalid values produce an error listing valid options. Shell completion is available for both keys and values.

For services, workers, tools, and aliases, use the dedicated commands instead:

- `frank add <service>` / `frank remove <service>`
- `frank worker queue` / `frank worker schedule`
- `frank tool add <tool>`
- Edit aliases directly in `frank.yaml` (see [`docs/shell.md`](shell.md#custom-aliases))

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
