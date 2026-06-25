package config

import (
	"os"
	"strings"
	"testing"
)

func TestDevIsEnabled(t *testing.T) {
	tr, fa := true, false

	cases := []struct {
		name string
		in   *bool
		want bool
	}{
		{"nil defaults true", nil, true},
		{"explicit true", &tr, true},
		{"explicit false", &fa, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := (Dev{Enabled: c.in}).IsEnabled(); got != c.want {
				t.Errorf("IsEnabled() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestDevEffectiveCommand(t *testing.T) {
	cases := []struct {
		pm   string
		want string
	}{
		{"npm", "[ -d node_modules ] || npm install; npm run dev"},
		{"pnpm", "[ -d node_modules ] || pnpm install; pnpm dev"},
		{"bun", "[ -d node_modules ] || bun install; bun run dev"},
		{"", "[ -d node_modules ] || npm install; npm run dev"}, // unset → npm
	}
	for _, c := range cases {
		t.Run(c.pm, func(t *testing.T) {
			if got := (Dev{}).EffectiveCommand(c.pm); got != c.want {
				t.Errorf("EffectiveCommand(%q) = %q, want %q", c.pm, got, c.want)
			}
		})
	}
}

func TestDevEffectiveCommandVerbatim(t *testing.T) {
	d := Dev{Command: "vite --host"}
	// Explicit command is trusted verbatim, ignoring the package manager.
	if got := d.EffectiveCommand("pnpm"); got != "vite --host" {
		t.Errorf("EffectiveCommand = %q, want verbatim override", got)
	}
}

// New() must NOT materialize Dev.Enabled — keeping it nil avoids changing the
// round-tripped frank.yaml. IsEnabled still resolves it to true.
func TestDevDefaultNotMaterialized(t *testing.T) {
	cfg := New()
	if cfg.Dev.Enabled != nil {
		t.Errorf("Dev.Enabled should stay nil after defaults, got %v", *cfg.Dev.Enabled)
	}

	if !cfg.Dev.IsEnabled() {
		t.Error("Dev.IsEnabled() should default true")
	}
}

func TestDevUnknownKeyWarning(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `
version: 1
dev:
  enabled: true
  futureThing: yes
`)

	r, w, _ := os.Pipe()
	oldStderr := os.Stderr
	os.Stderr = w
	_, err := Load(dir)

	w.Close()

	os.Stderr = oldStderr

	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)

	if out := string(buf[:n]); !strings.Contains(out, "futureThing") {
		t.Errorf("expected warning for unknown dev key, got: %q", out)
	}
}
