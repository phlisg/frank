package compose

import (
	"strings"
	"testing"

	"github.com/phlisg/frank/internal/config"
)

func baseDevCfg() *config.Config {
	return &config.Config{
		PHP:      config.PHP{Version: "8.5", Runtime: "frankenphp"},
		Laravel:  config.Laravel{Version: "latest"},
		Services: []string{"pgsql", "mailpit"},
	}
}

// The emitted laravel.vite port must track the vitePort argument, not a
// hardcoded 5173:5173 — otherwise sibling worktrees collide on the host port.
func TestGenerate_VitePortFollowsArg(t *testing.T) {
	g := newTestGenerator(t)

	// Non-worktree: vitePort 5173.
	out, err := g.Generate(baseDevCfg(), "myapp", "myapp", false, 5173)
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	if !strings.Contains(out, "laravel.vite:") {
		t.Fatalf("expected laravel.vite service:\n%s", out)
	}

	if !strings.Contains(out, "5173:5173") {
		t.Errorf("expected 5173:5173 vite mapping:\n%s", out)
	}

	// Worktree mode: an ephemeral port in 5174–5199.
	out, err = g.Generate(baseDevCfg(), "wt", "wt", true, 5187)
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	if !strings.Contains(out, "5187:5173") {
		t.Errorf("expected 5187:5173 vite mapping:\n%s", out)
	}

	if strings.Contains(out, "5173:5173") {
		t.Errorf("worktree output must not bind host 5173:\n%s", out)
	}
}

// laravel.test no longer publishes 5173 — it moved to laravel.vite.
func TestGenerate_VitePortNotOnLaravelTest(t *testing.T) {
	g := newTestGenerator(t)

	out, err := g.Generate(baseDevCfg(), "myapp", "myapp", false, 5173)
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}
	// laravel.test section ends before laravel.vite; its slice must lack 5173.
	ltStart := strings.Index(out, "laravel.test:")
	viteStart := strings.Index(out, "laravel.vite:")

	if ltStart < 0 || viteStart < 0 || viteStart < ltStart {
		t.Fatalf("unexpected service ordering:\n%s", out)
	}

	if strings.Contains(out[ltStart:viteStart], "5173") {
		t.Errorf("laravel.test must not bind 5173:\n%s", out[ltStart:viteStart])
	}
}

// dev.enabled: false → no laravel.vite service, port unmapped.
func TestGenerate_DevDisabledOmitsVite(t *testing.T) {
	g := newTestGenerator(t)
	cfg := baseDevCfg()
	off := false
	cfg.Dev = config.Dev{Enabled: &off}

	out, err := g.Generate(cfg, "myapp", "myapp", false, 5173)
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	if strings.Contains(out, "laravel.vite:") {
		t.Errorf("dev disabled: expected no laravel.vite:\n%s", out)
	}

	if strings.Contains(out, "5173") {
		t.Errorf("dev disabled: expected no 5173 mapping:\n%s", out)
	}
}

// The dev command derives from the package manager.
func TestGenerate_ViteCommandFromPackageManager(t *testing.T) {
	g := newTestGenerator(t)
	cfg := baseDevCfg()
	cfg.Node = config.Node{PackageManager: "pnpm"}

	out, err := g.Generate(cfg, "myapp", "myapp", false, 5173)
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	if !strings.Contains(out, "pnpm dev") {
		t.Errorf("expected pnpm dev command:\n%s", out)
	}
}
