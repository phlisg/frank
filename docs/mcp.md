# MCP Integration

[← Back to README](../README.md)

Frank includes a built-in [Model Context Protocol](https://modelcontextprotocol.io/) (MCP) server that lets AI coding assistants (Claude Code, Cursor, Windsurf, etc.) interact with your Docker environment directly.

## Setup

`frank generate` automatically creates a `.mcp.json` file in your project root:

```json
{
  "mcpServers": {
    "frank": {
      "command": "frank",
      "args": ["mcp"]
    }
  }
}
```

IDEs that support project-scoped MCP servers (Claude Code, Cursor) will discover this file automatically. No manual configuration needed.

## Available Tools

| Tool | Description |
|------|-------------|
| `frank_status` | Container status, health, and port mappings as JSON |
| `frank_config` | Fully resolved `frank.yaml` configuration as JSON |
| `frank_logs` | Tail container logs (all services or a specific one) |
| `frank_exec` | Run a command inside a container (artisan, composer, npm, etc.) |
| `frank_worktrees` | List, create, remove git worktrees, or clone a database into one — see below |

## Usage

Once the MCP server is connected, your AI assistant can use these tools instead of shelling out to `docker compose`. For example, asking "check container status" will use `frank_status` rather than running `frank compose ps`.

The first time you use it in Claude Code, you may need to approve the MCP server. Run `/mcp` in Claude Code to verify the connection.

## Worktree Support

When running from a git worktree, `frank_status` includes a `worktree` object:

```json
{
  "services": [...],
  "worktree": {
    "active": true,
    "vitePort": 5191
  }
}
```

This tells the AI assistant that ports are ephemeral and provides the deterministic Vite port.

### `frank_worktrees` Tool

Manages git worktrees programmatically. Takes an `action` parameter:

| Action | Parameters | Description |
|--------|-----------|-------------|
| `list` | — | Returns all linked worktrees with branch, status, and ports as JSON |
| `create` | `branch` (required), `seedDatabase` (optional, default `true`) | Creates a new worktree as a sibling directory (`../<project>-<kebab-branch>`). By default marks it to receive a clone of the main project's database on its first `frank up` — pass `seedDatabase: false` for a worktree that only needs code |
| `remove` | `path` (required) | Full teardown: containers, named volumes, images, the worktree directory and the branch |
| `clone-db` | `path` (required) | Clones the main project's database into an existing worktree immediately. For worktrees created before seeding existed, or to refresh stale data. Both projects must be running |

Example response from `list`:

```json
[
  {
    "path": "/home/user/code/myapp-feature-auth",
    "branch": "feature/auth",
    "hasFrank": true,
    "status": "running (3/3)",
    "ports": "web:32771  vite:32773  pgsql:32768"
  }
]
```

`ports` is a labelled listing of the worktree's published host ports, built from
its running services only: `web` is `laravel.test`, `vite` is `laravel.vite`, and
every other service — including any declared under `extra_services` — keeps its
Compose service name. TCP publishers only; a service publishing several ports
collapses into one entry (`web:32771,32770`), and a service with no published
port is omitted. Ordering is `web`, `vite`, then alphabetical.

This lets AI assistants create isolated worktrees, start/stop their containers, and clean them up when done — without shelling out to git or docker.

`remove` is destructive in a way a plain `git worktree remove` is not: it deletes the worktree's database volume along with everything else. See [Removing a Worktree](worktrees.md#removing-a-worktree).

## Manual Use

The MCP server runs over stdio and is not intended for direct use:

```bash
frank mcp  # starts stdio JSON-RPC — used by IDEs, not humans
```

The command is hidden from `frank --help` since it's an integration point, not a user-facing command.
