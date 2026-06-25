package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phlisg/frank/internal/config"
)

// TestFlattenDockerfile asserts that eject's flatten step rewrites
// .frank/Dockerfile into a self-contained form: no FROM frank/runtime, the
// runtime's real base FROM, and (frankenphp only) the Caddyfile COPY layer.
func TestFlattenDockerfile(t *testing.T) {
	tests := []struct {
		name          string
		cfg           *config.Config
		wantFrom      string
		wantCaddyCopy bool
	}{
		{
			name: "frankenphp",
			cfg: &config.Config{
				PHP:      config.PHP{Version: "8.5", Runtime: "frankenphp"},
				Services: []string{"pgsql", "mailpit"},
			},
			wantFrom:      "FROM dunglas/frankenphp",
			wantCaddyCopy: true,
		},
		{
			name: "fpm",
			cfg: &config.Config{
				PHP:      config.PHP{Version: "8.4", Runtime: "fpm"},
				Services: []string{"mysql", "redis"},
			},
			wantFrom:      "FROM ubuntu:24.04",
			wantCaddyCopy: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			frankDir := filepath.Join(dir, ".frank")
			if err := os.MkdirAll(frankDir, 0755); err != nil {
				t.Fatal(err)
			}
			// Placeholder Caddyfile + thin Dockerfile to be overwritten.
			if err := os.WriteFile(filepath.Join(frankDir, "Caddyfile"), []byte("# placeholder\n"), 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(frankDir, "Dockerfile"), []byte("FROM frank/runtime:thin\n"), 0644); err != nil {
				t.Fatal(err)
			}

			if err := flattenDockerfile(dir, tt.cfg); err != nil {
				t.Fatalf("flattenDockerfile: %v", err)
			}

			out, err := os.ReadFile(filepath.Join(frankDir, "Dockerfile"))
			if err != nil {
				t.Fatal(err)
			}
			got := string(out)

			if strings.Contains(got, "FROM frank/runtime") {
				t.Errorf("flattened Dockerfile still contains FROM frank/runtime")
			}
			if !strings.Contains(got, tt.wantFrom) {
				t.Errorf("flattened Dockerfile missing %q\n---\n%s", tt.wantFrom, got)
			}
			hasCaddy := strings.Contains(got, "COPY .frank/Caddyfile")
			if hasCaddy != tt.wantCaddyCopy {
				t.Errorf("Caddyfile COPY present=%v, want %v", hasCaddy, tt.wantCaddyCopy)
			}
		})
	}
}
