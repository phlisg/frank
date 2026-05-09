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

No config changes needed — Frank detects worktrees automatically.

## Managing Worktrees

### Interactive TUI

```bash
frank worktree list
```

Opens an interactive list of all linked worktrees with their container status and ports. Keybindings:

| Key | Action |
|-----|--------|
| `c` | Create a new worktree |
| `o` | Open in browser |
| `u` | Start containers (`frank up -d`) |
| `d` | Stop containers (`frank down`) |
| `l` | Tail container logs |
| `g` | Regenerate `.frank/` |
| `e` | Open in `$EDITOR` |
| `r` | Remove worktree (confirms first) |
| `/` | Filter by branch name |
| `q` | Quit |

### CLI Commands

```bash
frank worktree create <branch>    # Create a new worktree
frank worktree remove <path>      # Tear down containers, remove worktree + branch
frank worktree list               # Interactive TUI
```

`frank worktree remove` handles cleanup: stops containers, removes the worktree directory, and deletes the git branch.

## MCP Integration

The `frank_worktrees` MCP tool exposes worktree management to AI assistants. See [`docs/mcp.md`](mcp.md) for details.
