package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/worktreelist"
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

func (h *handlers) handleWorktrees(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	action := req.GetString("action", "list")

	switch action {
	case "list":
		items, err := worktreelist.Discover(h.dir)
		if err != nil {
			return errorResult(fmt.Sprintf("discover: %v", err)), nil
		}

		type wtJSON struct {
			Path     string `json:"path"`
			Branch   string `json:"branch"`
			HasFrank bool   `json:"hasFrank"`
			Status   string `json:"status"`
			Ports    string `json:"ports,omitempty"`
		}

		var result []wtJSON
		for _, item := range items {
			result = append(result, wtJSON{
				Path:     item.Path,
				Branch:   item.Branch,
				HasFrank: item.HasFrank,
				Status:   item.StatusLabel(),
				Ports:    item.PortSummary(),
			})
		}

		b, _ := json.MarshalIndent(result, "", "  ")

		return textResult(string(b)), nil

	case "remove":
		path := req.GetString("path", "")
		if path == "" {
			return errorResult("path required for remove"), nil
		}

		absPath, err := filepath.Abs(path)
		if err != nil {
			return errorResult(fmt.Sprintf("resolve path: %v", err)), nil
		}

		items, _ := worktreelist.Discover(h.dir)

		var branch string

		for _, item := range items {
			if item.Path == absPath {
				branch = item.Branch
				break
			}
		}

		if err := worktreelist.RemoveWorktree(absPath, branch); err != nil {
			return errorResult(fmt.Sprintf("remove: %v", err)), nil
		}

		return textResult(fmt.Sprintf("removed worktree %s", absPath)), nil

	case "create":
		branch := req.GetString("branch", "")
		if branch == "" {
			return errorResult("branch required for create"), nil
		}

		projectName := config.ProjectName(h.dir)
		kebab := worktreelist.BranchToKebab(branch)
		parentDir := filepath.Dir(h.dir)

		wtPath := filepath.Join(parentDir, projectName+"-"+kebab)
		if err := worktreelist.CreateWorktree(h.dir, wtPath, branch); err != nil {
			return errorResult(fmt.Sprintf("create: %v", err)), nil
		}

		return textResult(fmt.Sprintf("created worktree at %s", wtPath)), nil

	default:
		return errorResult(fmt.Sprintf("unknown action: %s", action)), nil
	}
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
