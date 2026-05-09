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
| `frank_worktrees` | List, create, or remove git worktrees — see below |

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
| `create` | `branch` (required) | Creates a new worktree as a sibling directory (`../<project>-<kebab-branch>`) |
| `remove` | `path` (required) | Tears down containers, removes the worktree directory, and deletes the branch |

Example response from `list`:

```json
[
  {
    "path": "/home/user/code/myapp-feature-auth",
    "branch": "feature/auth",
    "hasFrank": true,
    "status": "running (3/3)",
    "ports": ":32771 :32768 :5191"
  }
]
```

This lets AI assistants create isolated worktrees, start/stop their containers, and clean them up when done — without shelling out to git or docker.

## Manual Use

The MCP server runs over stdio and is not intended for direct use:

```bash
frank mcp  # starts stdio JSON-RPC — used by IDEs, not humans
```

The command is hidden from `frank --help` since it's an integration point, not a user-facing command.
