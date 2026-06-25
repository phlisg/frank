package cmd

import (
	"strings"
	"testing"

	"github.com/phlisg/frank/internal/config"
	"gopkg.in/yaml.v3"
)

func TestApplyWorkersFromInitNone(t *testing.T) {
	cfg := config.New()
	applyWorkersFromInit(cfg, false, 0)

	if cfg.Workers.Schedule {
		t.Error("Schedule should be false")
	}

	if len(cfg.Workers.Queue) != 0 {
		t.Errorf("Queue len = %d, want 0", len(cfg.Workers.Queue))
	}
}

func TestApplyWorkersFromInitScheduleOnly(t *testing.T) {
	cfg := config.New()
	applyWorkersFromInit(cfg, true, 0)

	if !cfg.Workers.Schedule {
		t.Error("Schedule should be true")
	}

	if len(cfg.Workers.Queue) != 0 {
		t.Errorf("Queue len = %d, want 0", len(cfg.Workers.Queue))
	}
}

func TestApplyWorkersFromInitQueueOnly(t *testing.T) {
	cfg := config.New()
	applyWorkersFromInit(cfg, false, 3)

	if cfg.Workers.Schedule {
		t.Error("Schedule should be false")
	}

	if len(cfg.Workers.Queue) != 1 {
		t.Fatalf("Queue len = %d, want 1", len(cfg.Workers.Queue))
	}

	p := cfg.Workers.Queue[0]
	if p.Name != "default" || p.Count != 3 {
		t.Errorf("pool = %+v, want name=default count=3", p)
	}

	if len(p.Queues) != 1 || p.Queues[0] != "default" {
		t.Errorf("Queues = %v, want [default]", p.Queues)
	}
}

func TestApplyWorkersFromInitBoth(t *testing.T) {
	cfg := config.New()
	applyWorkersFromInit(cfg, true, 2)

	if !cfg.Workers.Schedule {
		t.Error("Schedule should be true")
	}

	if len(cfg.Workers.Queue) != 1 || cfg.Workers.Queue[0].Count != 2 {
		t.Errorf("Queue = %+v", cfg.Workers.Queue)
	}
}

// stripComments returns only the non-comment lines from the marshalled output,
// so tests can check the active YAML without matching the reference comment block.
func stripComments(s string) string {
	var lines []string

	for _, line := range strings.Split(s, "\n") {
		if !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}

	return strings.Join(lines, "\n")
}

func TestMarshalConfigOmitsEmptyWorkers(t *testing.T) {
	cfg := config.New()
	// Explicitly clear workers to test omission behaviour.
	cfg.Workers = config.Workers{}

	out, err := marshalConfig(cfg)
	if err != nil {
		t.Fatalf("marshalConfig: %v", err)
	}

	active := stripComments(out)
	if strings.Contains(active, "workers:") {
		t.Errorf("expected no workers key for empty workers, got:\n%s", out)
	}
}

func TestMarshalConfigEmitsDefaultWorkers(t *testing.T) {
	cfg := config.New()

	out, err := marshalConfig(cfg)
	if err != nil {
		t.Fatalf("marshalConfig: %v", err)
	}

	if !strings.Contains(out, "workers:") {
		t.Errorf("expected workers key for default config, got:\n%s", out)
	}

	if !strings.Contains(out, "schedule: true") {
		t.Errorf("expected schedule: true in default config, got:\n%s", out)
	}
}

func TestMarshalConfigOmitsDefaultNode(t *testing.T) {
	cfg := config.New() // Node.PackageManager = "npm" (default)

	out, err := marshalConfig(cfg)
	if err != nil {
		t.Fatalf("marshalConfig: %v", err)
	}

	active := stripComments(out)
	if strings.Contains(active, "node:") {
		t.Errorf("expected no node key for default npm, got:\n%s", out)
	}
}

func TestMarshalConfigEmitsNonDefaultNode(t *testing.T) {
	cfg := config.New()
	cfg.Node.PackageManager = "pnpm"

	out, err := marshalConfig(cfg)
	if err != nil {
		t.Fatalf("marshalConfig: %v", err)
	}

	if !strings.Contains(out, "node:") {
		t.Errorf("expected node key for pnpm, got:\n%s", out)
	}

	if !strings.Contains(out, "packageManager: pnpm") {
		t.Errorf("expected packageManager: pnpm, got:\n%s", out)
	}
}

func TestMarshalConfigRoundtripPreservesAllFields(t *testing.T) {
	httpsOff := false
	cfg := config.New()
	cfg.Aliases = map[string]config.Alias{
		"migrate": {Cmd: "php artisan migrate"},
		"tinker":  {Cmd: "php artisan tinker"},
	}
	cfg.Server = config.Server{HTTPS: &httpsOff, Port: 8443}
	cfg.Node.PackageManager = "pnpm"
	applyWorkersFromInit(cfg, true, 3)
	cfg.Tools = []string{"pint", "phpstan"}

	out, err := marshalConfig(cfg)
	if err != nil {
		t.Fatalf("marshalConfig: %v", err)
	}

	// Parse back and verify nothing was dropped.
	var roundtripped config.Config

	clean := stripComments(out)
	if err := yaml.Unmarshal([]byte(clean), &roundtripped); err != nil {
		t.Fatalf("unmarshal roundtripped yaml: %v", err)
	}

	// Aliases
	if len(roundtripped.Aliases) != 2 {
		t.Errorf("aliases count = %d, want 2", len(roundtripped.Aliases))
	}

	if a, ok := roundtripped.Aliases["migrate"]; !ok || a.Cmd != "php artisan migrate" {
		t.Errorf("alias migrate = %+v", roundtripped.Aliases["migrate"])
	}

	// Server
	if roundtripped.Server.HTTPS == nil || *roundtripped.Server.HTTPS != false {
		t.Errorf("server.https = %v, want false", roundtripped.Server.HTTPS)
	}

	if roundtripped.Server.Port != 8443 {
		t.Errorf("server.port = %d, want 8443", roundtripped.Server.Port)
	}

	// Node
	if roundtripped.Node.PackageManager != "pnpm" {
		t.Errorf("node.packageManager = %q, want pnpm", roundtripped.Node.PackageManager)
	}

	// Workers
	if !roundtripped.Workers.Schedule {
		t.Error("workers.schedule should be true")
	}

	if len(roundtripped.Workers.Queue) != 1 || roundtripped.Workers.Queue[0].Count != 3 {
		t.Errorf("workers.queue = %+v", roundtripped.Workers.Queue)
	}

	// Tools
	if len(roundtripped.Tools) != 2 {
		t.Errorf("tools = %v, want [pint phpstan]", roundtripped.Tools)
	}
}

func TestMarshalConfigEmitsWorkers(t *testing.T) {
	cfg := config.New()
	applyWorkersFromInit(cfg, true, 2)

	out, err := marshalConfig(cfg)
	if err != nil {
		t.Fatalf("marshalConfig: %v", err)
	}

	if !strings.Contains(out, "workers:") {
		t.Errorf("expected workers key, got:\n%s", out)
	}

	if !strings.Contains(out, "schedule: true") {
		t.Errorf("expected schedule: true, got:\n%s", out)
	}

	if !strings.Contains(out, "count: 2") {
		t.Errorf("expected count: 2, got:\n%s", out)
	}
	// omitempty fields should not appear
	for _, unwanted := range []string{"tries:", "timeout:", "memory:", "sleep:", "backoff:"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("did not expect %q in output:\n%s", unwanted, out)
		}
	}
}
