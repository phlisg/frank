package mcp

import "github.com/mark3labs/mcp-go/mcp"

var statusTool = mcp.NewTool("frank_status",
	mcp.WithDescription("Container health overview — shows state, health, and ports for all services"),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithDestructiveHintAnnotation(false),
)

var configTool = mcp.NewTool("frank_config",
	mcp.WithDescription("Resolved frank.yaml configuration as JSON"),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithDestructiveHintAnnotation(false),
)

var logsTool = mcp.NewTool("frank_logs",
	mcp.WithDescription("Tail service logs from running containers"),
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

var execTool = mcp.NewTool("frank_exec",
	mcp.WithDescription("Run a command inside a container"),
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
