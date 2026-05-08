package mcp

import (
	"github.com/mark3labs/mcp-go/server"
	"github.com/phlisg/frank/internal/config"
)

func Serve(client dockerClient, cfg *config.Config, version string) error {
	s := server.NewMCPServer("frank", version)

	h := &handlers{client: client, cfg: cfg}

	s.AddTool(statusTool, h.handleStatus)
	s.AddTool(configTool, h.handleConfig)
	s.AddTool(logsTool, h.handleLogs)
	s.AddTool(execTool, h.handleExec)

	return server.ServeStdio(s)
}
