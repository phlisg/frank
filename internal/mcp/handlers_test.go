package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/phlisg/frank/internal/config"
)

type mockDocker struct {
	runQuietFn  func(args ...string) (string, error)
	execQuietFn func(service string, command ...string) (string, error)
}

func (m *mockDocker) RunQuiet(args ...string) (string, error) {
	return m.runQuietFn(args...)
}

func (m *mockDocker) ExecQuiet(service string, command ...string) (string, error) {
	return m.execQuietFn(service, command...)
}

func makeRequest(args map[string]any) mcplib.CallToolRequest {
	req := mcplib.CallToolRequest{}
	req.Params.Arguments = args
	return req
}

func contentText(t *testing.T, result *mcplib.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("expected at least one content block")
	}
	tc, ok := result.Content[0].(mcplib.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	return tc.Text
}

func TestHandleStatus_Success(t *testing.T) {
	line1 := `{"Command":"\"start-container\"","CreatedAt":"2025-01-01 00:00:00 +0000 UTC","Health":"","ID":"abc123","Image":"frank-test-laravel.test","Name":"laravel.test","Networks":"frank","Ports":"0.0.0.0:443->443/tcp","Project":"test","Publishers":[{"URL":"0.0.0.0","TargetPort":443,"PublishedPort":443,"Protocol":"tcp"}],"Service":"laravel.test","State":"running","Status":"Up 1 hour"}`
	line2 := `{"Command":"\"docker-entrypoint…\"","CreatedAt":"2025-01-01 00:00:00 +0000 UTC","Health":"healthy","ID":"def456","Image":"postgres:17","Name":"pgsql","Networks":"frank","Ports":"0.0.0.0:5432->5432/tcp","Project":"test","Publishers":[{"URL":"0.0.0.0","TargetPort":5432,"PublishedPort":5432,"Protocol":"tcp"}],"Service":"pgsql","State":"running","Status":"Up 1 hour (healthy)"}`

	h := &handlers{
		client: &mockDocker{
			runQuietFn: func(args ...string) (string, error) {
				return line1 + "\n" + line2, nil
			},
		},
	}

	result, err := h.handleStatus(context.Background(), mcplib.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected IsError=false")
	}

	text := contentText(t, result)

	var response map[string]any
	if err := json.Unmarshal([]byte(text), &response); err != nil {
		t.Fatalf("failed to parse result JSON: %v", err)
	}

	svcList, ok := response["services"].([]any)
	if !ok {
		t.Fatal("expected services array")
	}
	if len(svcList) != 2 {
		t.Fatalf("expected 2 services, got %d", len(svcList))
	}

	// No worktree key when dir is empty (not a worktree).
	if _, has := response["worktree"]; has {
		t.Error("worktree key should be absent for non-worktree")
	}

	services := make([]map[string]any, len(svcList))
	for i, s := range svcList {
		services[i] = s.(map[string]any)
	}

	if services[0]["Name"] != "laravel.test" {
		t.Errorf("expected Name=laravel.test, got %v", services[0]["Name"])
	}
	if services[0]["State"] != "running" {
		t.Errorf("expected State=running, got %v", services[0]["State"])
	}
	if services[0]["Health"] != "" {
		t.Errorf("expected Health='', got %v", services[0]["Health"])
	}
	publishers, ok := services[0]["Publishers"].([]any)
	if !ok || len(publishers) == 0 {
		t.Error("expected non-empty Publishers for laravel.test")
	}

	if services[1]["Name"] != "pgsql" {
		t.Errorf("expected Name=pgsql, got %v", services[1]["Name"])
	}
	if services[1]["Health"] != "healthy" {
		t.Errorf("expected Health=healthy, got %v", services[1]["Health"])
	}
}

func TestHandleStatus_DockerError(t *testing.T) {
	h := &handlers{
		client: &mockDocker{
			runQuietFn: func(args ...string) (string, error) {
				return "", errors.New("docker daemon not running")
			},
		},
	}

	result, err := h.handleStatus(context.Background(), mcplib.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true")
	}

	text := contentText(t, result)
	if text == "" {
		t.Error("expected non-empty error message")
	}
}

func TestHandleConfig(t *testing.T) {
	cfg := &config.Config{
		Version:  1,
		PHP:      config.PHP{Version: "8.4", Runtime: "frankenphp"},
		Services: []string{"pgsql", "mailpit"},
	}
	h := &handlers{cfg: cfg}

	result, err := h.handleConfig(context.Background(), mcplib.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected IsError=false")
	}

	text := contentText(t, result)

	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("failed to parse config JSON: %v", err)
	}

	php, ok := parsed["PHP"].(map[string]any)
	if !ok {
		t.Fatal("expected PHP key in config")
	}
	if php["Version"] != "8.4" {
		t.Errorf("expected PHP.Version=8.4, got %v", php["Version"])
	}
	if php["Runtime"] != "frankenphp" {
		t.Errorf("expected PHP.Runtime=frankenphp, got %v", php["Runtime"])
	}

	services, ok := parsed["Services"].([]any)
	if !ok {
		t.Fatal("expected Services array in config")
	}
	if len(services) != 2 {
		t.Errorf("expected 2 services, got %d", len(services))
	}
}

func TestHandleLogs_AllServices(t *testing.T) {
	var captured []string
	h := &handlers{
		client: &mockDocker{
			runQuietFn: func(args ...string) (string, error) {
				captured = args
				return "log output here", nil
			},
		},
	}

	req := makeRequest(nil)
	result, err := h.handleLogs(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected IsError=false")
	}

	// Default: logs --tail 50
	if len(captured) != 3 {
		t.Fatalf("expected 3 args, got %d: %v", len(captured), captured)
	}
	if captured[0] != "logs" || captured[1] != "--tail" || captured[2] != "50" {
		t.Errorf("expected [logs --tail 50], got %v", captured)
	}

	text := contentText(t, result)
	if text != "log output here" {
		t.Errorf("expected log output, got %q", text)
	}
}

func TestHandleLogs_SpecificService(t *testing.T) {
	var captured []string
	h := &handlers{
		client: &mockDocker{
			runQuietFn: func(args ...string) (string, error) {
				captured = args
				return "pgsql logs", nil
			},
		},
	}

	req := makeRequest(map[string]any{
		"service": "pgsql",
		"lines":   float64(100),
	})
	result, err := h.handleLogs(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected IsError=false")
	}

	if len(captured) != 4 {
		t.Fatalf("expected 4 args, got %d: %v", len(captured), captured)
	}
	if captured[0] != "logs" || captured[1] != "--tail" || captured[2] != "100" || captured[3] != "pgsql" {
		t.Errorf("expected [logs --tail 100 pgsql], got %v", captured)
	}
}

func TestHandleExec_Default(t *testing.T) {
	var capturedService string
	var capturedCmd []string
	h := &handlers{
		client: &mockDocker{
			execQuietFn: func(service string, command ...string) (string, error) {
				capturedService = service
				capturedCmd = command
				return "migration output", nil
			},
		},
	}

	req := makeRequest(map[string]any{
		"command": []any{"php", "artisan", "migrate"},
	})
	result, err := h.handleExec(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected IsError=false")
	}

	if capturedService != "laravel.test" {
		t.Errorf("expected service=laravel.test, got %q", capturedService)
	}
	if len(capturedCmd) != 3 || capturedCmd[0] != "php" || capturedCmd[1] != "artisan" || capturedCmd[2] != "migrate" {
		t.Errorf("expected [php artisan migrate], got %v", capturedCmd)
	}

	text := contentText(t, result)
	if text != "migration output" {
		t.Errorf("expected 'migration output', got %q", text)
	}
}

func TestHandleExec_CustomService(t *testing.T) {
	var capturedService string
	var capturedCmd []string
	h := &handlers{
		client: &mockDocker{
			execQuietFn: func(service string, command ...string) (string, error) {
				capturedService = service
				capturedCmd = command
				return "psql output", nil
			},
		},
	}

	req := makeRequest(map[string]any{
		"command": []any{"psql"},
		"service": "pgsql",
	})
	result, err := h.handleExec(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected IsError=false")
	}

	if capturedService != "pgsql" {
		t.Errorf("expected service=pgsql, got %q", capturedService)
	}
	if len(capturedCmd) != 1 || capturedCmd[0] != "psql" {
		t.Errorf("expected [psql], got %v", capturedCmd)
	}
}

func TestHandleExec_EmptyCommand(t *testing.T) {
	h := &handlers{
		client: &mockDocker{
			execQuietFn: func(service string, command ...string) (string, error) {
				t.Fatal("ExecQuiet should not be called for empty command")
				return "", nil
			},
		},
	}

	req := makeRequest(map[string]any{
		"command": []any{},
	})
	result, err := h.handleExec(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for empty command")
	}

	text := contentText(t, result)
	if text == "" {
		t.Error("expected non-empty error message")
	}
}

func TestHandleExec_MissingCommand(t *testing.T) {
	h := &handlers{
		client: &mockDocker{
			execQuietFn: func(service string, command ...string) (string, error) {
				t.Fatal("ExecQuiet should not be called when command is missing")
				return "", nil
			},
		},
	}

	req := makeRequest(nil)
	result, err := h.handleExec(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for missing command")
	}

	text := contentText(t, result)
	if text == "" {
		t.Error("expected non-empty error message")
	}
}
