package cmd

import (
	"strings"
	"testing"

	"github.com/phlisg/frank/internal/config"
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

func TestMarshalConfigOmitsEmptyWorkers(t *testing.T) {
	cfg := config.New()
	out, err := marshalConfig(cfg)
	if err != nil {
		t.Fatalf("marshalConfig: %v", err)
	}
	if strings.Contains(out, "workers:") {
		t.Errorf("expected no workers key for empty workers, got:\n%s", out)
	}
}

func TestMarshalConfigOmitsDefaultNode(t *testing.T) {
	cfg := config.New() // Node.PackageManager = "npm" (default)
	out, err := marshalConfig(cfg)
	if err != nil {
		t.Fatalf("marshalConfig: %v", err)
	}
	if strings.Contains(out, "node:") {
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
