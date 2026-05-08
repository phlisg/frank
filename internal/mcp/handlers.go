package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/phlisg/frank/internal/config"
)

type handlers struct {
	client dockerClient
	cfg    *config.Config
	dir    string
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.NewTextContent(text)},
	}
}

func errorResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.NewTextContent(msg)},
		IsError: true,
	}
}

func (h *handlers) handleStatus(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	out, err := h.client.RunQuiet("ps", "--format", "json")
	if err != nil {
		return errorResult(fmt.Sprintf(`{"error": %q}`, err.Error())), nil
	}

	// docker compose ps --format json outputs one JSON object per line
	var services []json.RawMessage
	for _, line := range splitLines(out) {
		if line == "" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			continue
		}
		compact := map[string]any{
			"Name":       obj["Name"],
			"State":      obj["State"],
			"Health":     obj["Health"],
			"Publishers": obj["Publishers"],
		}
		b, _ := json.Marshal(compact)
		services = append(services, b)
	}

	response := map[string]any{
		"services": services,
	}
	if config.IsWorktree(h.dir) {
		projectName := config.ProjectName(h.dir)
		response["worktree"] = map[string]any{
			"active":   true,
			"vitePort": config.ViteWorktreePort(projectName),
		}
	}

	result, _ := json.MarshalIndent(response, "", "  ")
	return textResult(string(result)), nil
}

func (h *handlers) handleConfig(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(h.cfg, "", "  ")
	if err != nil {
		return errorResult(fmt.Sprintf(`{"error": %q}`, err.Error())), nil
	}
	return textResult(string(b)), nil
}

func (h *handlers) handleLogs(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	service := req.GetString("service", "")
	lines := req.GetInt("lines", 50)

	args := []string{"logs", "--tail", strconv.Itoa(lines)}
	if service != "" {
		args = append(args, service)
	}

	out, err := h.client.RunQuiet(args...)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	return textResult(out), nil
}

func (h *handlers) handleExec(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	command, err := req.RequireStringSlice("command")
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if len(command) == 0 {
		return errorResult("command array must not be empty"), nil
	}

	service := req.GetString("service", "laravel.test")

	out, err := h.client.ExecQuiet(service, command...)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	return textResult(out), nil
}

// splitLines splits a string into non-empty lines.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
