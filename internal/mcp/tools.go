package mcp

import "github.com/mark3labs/mcp-go/mcp"

var statusTool = mcp.NewTool("frank_status",
	mcp.WithDescription("Check container status — use this instead of running docker compose ps or frank compose ps. Returns state, health, and ports for all services as structured JSON."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithDestructiveHintAnnotation(false),
)

var configTool = mcp.NewTool("frank_config",
	mcp.WithDescription("Read the project's frank.yaml configuration — use this instead of reading frank.yaml directly. Returns the fully resolved config (with defaults applied) as JSON."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithDestructiveHintAnnotation(false),
)

var logsTool = mcp.NewTool("frank_logs",
	mcp.WithDescription("Tail container logs — use this instead of docker compose logs. Returns recent log output for one or all services."),
	mcp.WithString("service",
		mcp.Description("Service name, e.g. laravel.test, pgsql — omit for all"),
	),
	mcp.WithNumber("lines",
		mcp.Description("Number of lines to tail"),
		mcp.DefaultNumber(50),
	),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithDestructiveHintAnnotation(false),
)

var worktreesTool = mcp.NewTool("frank_worktrees",
	mcp.WithDescription("List or manage git worktrees. Action 'list' returns all linked worktrees with container status and ports. Action 'remove' tears down containers and removes a worktree+branch. Action 'create' creates a new worktree as sibling directory."),
	mcp.WithString("action",
		mcp.Required(),
		mcp.Description("Action: list, remove, or create"),
	),
	mcp.WithString("path",
		mcp.Description("Worktree path — required for 'remove'"),
	),
	mcp.WithString("branch",
		mcp.Description("Branch name — required for 'create'"),
	),
)

var execTool = mcp.NewTool("frank_exec",
	mcp.WithDescription("Run a command inside a container — use this instead of docker compose exec or frank exec. Supports artisan, composer, npm, pest, and any other CLI tool."),
	mcp.WithArray("command",
		mcp.Required(),
		mcp.Description(`Command and arguments, e.g. ["php", "artisan", "migrate"]`),
		mcp.WithStringItems(),
	),
	mcp.WithString("service",
		mcp.Description("Target service — defaults to laravel.test"),
	),
	mcp.WithDestructiveHintAnnotation(false),
)
