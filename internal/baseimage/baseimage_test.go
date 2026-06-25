package baseimage

import (
	"os"
	"strings"
	"testing"

	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/template"
)

func newTestEngine() *template.Engine {
	// Engine prepends "templates/" internally, so root the FS at the frank root.
	return template.New(os.DirFS("../.."))
}

func TestTag(t *testing.T) {
	cases := []struct {
		name    string
		php     string
		runtime string
		want    string
	}{
		{"frankenphp", "8.5", "frankenphp", "frank/runtime:8.5-frankenphp-node24-pg17"},
		{"fpm", "8.4", "fpm", "frank/runtime:8.4-fpm-node24-pg17"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{PHP: config.PHP{Version: tc.php, Runtime: tc.runtime}}
			got := Tag(cfg)
			if got != tc.want {
				t.Fatalf("Tag = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHashStableAndSensitive(t *testing.T) {
	const in = "FROM dunglas/frankenphp:1-php8.5\nRUN echo hi\n"

	a := Hash(in)
	b := Hash(in)
	if a != b {
		t.Fatalf("Hash not stable: %q != %q", a, b)
	}
	if len(a) != 64 {
		t.Fatalf("Hash length = %d, want 64 hex chars", len(a))
	}

	if c := Hash(in + "x"); c == a {
		t.Fatalf("Hash did not change for different input")
	}
}

func TestRender(t *testing.T) {
	e := newTestEngine()

	cfg := &config.Config{PHP: config.PHP{Version: "8.5", Runtime: "frankenphp"}}
	out, err := Render(e, cfg)
	if err != nil {
		t.Fatalf("Render(frankenphp) error: %v", err)
	}
	if !strings.Contains(out, "dunglas/frankenphp:1-php8.5") {
		t.Fatalf("Render(frankenphp) missing PHP version interpolation:\n%s", out)
	}

	fpmCfg := &config.Config{PHP: config.PHP{Version: "8.4", Runtime: "fpm"}}
	fpmOut, err := Render(e, fpmCfg)
	if err != nil {
		t.Fatalf("Render(fpm) error: %v", err)
	}
	if !strings.Contains(fpmOut, "php8.4-fpm") {
		t.Fatalf("Render(fpm) missing PHP version interpolation:\n%s", fpmOut)
	}

	// Hash of a real render is deterministic and well-formed.
	if h := Hash(out); len(h) != 64 {
		t.Fatalf("Hash(render) length = %d, want 64", len(h))
	}
}
