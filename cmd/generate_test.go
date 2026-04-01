package cmd

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phlisg/frank/internal/config"
)

// update regenerates golden files when passed: go test ./cmd/ -update
var update = flag.Bool("update", false, "regenerate golden files in testdata/")

// TestMain sets TemplateFS to the real on-disk templates before any test runs.
// This mirrors what main.go does via embed, but without the embed dependency.
func TestMain(m *testing.M) {
	TemplateFS = os.DirFS("..")
	os.Exit(m.Run())
}

type integrationFixture struct {
	name  string
	cfg   *config.Config
	files []string // output files expected to exist
}

var integrationFixtures = []integrationFixture{
	{
		name: "frankenphp-pgsql-mailpit",
		cfg: &config.Config{
			PHP:      config.PHP{Version: "8.5", Runtime: "frankenphp"},
			Laravel:  config.Laravel{Version: "13.x"},
			Services: []string{"pgsql", "mailpit"},
		},
		files: []string{"compose.yaml", ".env", ".env.example", "Dockerfile", "Caddyfile"},
	},
	{
		name: "fpm-mysql-redis",
		cfg: &config.Config{
			PHP:      config.PHP{Version: "8.4", Runtime: "fpm"},
			Laravel:  config.Laravel{Version: "12.x"},
			Services: []string{"mysql", "redis"},
		},
		files: []string{"compose.yaml", ".env", ".env.example", "Dockerfile", "nginx.conf", "nginx.Dockerfile"},
	},
	{
		name: "frankenphp-sqlite",
		cfg: &config.Config{
			PHP:      config.PHP{Version: "8.5", Runtime: "frankenphp"},
			Laravel:  config.Laravel{Version: "13.x"},
			Services: []string{"sqlite"},
		},
		files: []string{"compose.yaml", ".env", ".env.example", "Dockerfile", "Caddyfile"},
	},
}

func TestGenerate_Integration(t *testing.T) {
	for _, fx := range integrationFixtures {
		t.Run(fx.name, func(t *testing.T) {
			// Named subdir so config.ProjectName(dir) returns fx.name (predictable).
			dir := filepath.Join(t.TempDir(), fx.name)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}

			if err := generate(fx.cfg, dir); err != nil {
				t.Fatalf("generate: %v", err)
			}

			goldenDir := filepath.Join("testdata", fx.name)

			for _, fname := range fx.files {
				data, err := os.ReadFile(filepath.Join(dir, fname))
				if err != nil {
					t.Errorf("output file %s missing: %v", fname, err)
					continue
				}
				got := string(data)
				goldenPath := filepath.Join(goldenDir, fname)

				if *update {
					if err := os.MkdirAll(goldenDir, 0755); err != nil {
						t.Fatalf("mkdir golden: %v", err)
					}
					if err := os.WriteFile(goldenPath, []byte(got), 0644); err != nil {
						t.Fatalf("write golden %s: %v", goldenPath, err)
					}
					t.Logf("updated golden: %s", goldenPath)
					continue
				}

				goldenBytes, err := os.ReadFile(goldenPath)
				if err != nil {
					t.Errorf("golden file %s missing — run: go test ./cmd/ -update\nerror: %v", goldenPath, err)
					continue
				}
				if got != string(goldenBytes) {
					t.Errorf("file %s differs from golden\n--- want ---\n%s\n--- got ---\n%s", fname, string(goldenBytes), got)
				}
			}

			checkInvariants(t, fx, dir)
		})
	}
}

// checkInvariants asserts properties that must hold regardless of golden content.
func checkInvariants(t *testing.T, fx integrationFixture, dir string) {
	t.Helper()

	env := readTestFile(t, dir, ".env")
	example := readTestFile(t, dir, ".env.example")

	// APP_KEY must be blank (never pre-filled by frank generate)
	if !strings.Contains(env, "APP_KEY=\n") {
		t.Error(".env: APP_KEY must be present with an empty value")
	}
	if !strings.Contains(example, "APP_KEY=\n") {
		t.Error(".env.example: APP_KEY must be present with an empty value")
	}

	// Sensitive values must not appear in .env.example
	for _, pair := range []struct{ key, badVal string }{
		{"DB_PASSWORD", "password"},
		{"REDIS_PASSWORD", "null"},
	} {
		if strings.Contains(example, pair.key+"="+pair.badVal) {
			t.Errorf(".env.example: %s must be redacted, found real value", pair.key)
		}
	}

	// pgsql-specific: DB_URL must be populated in .env, empty in .env.example
	if fx.cfg.Database() == "pgsql" {
		if !strings.Contains(env, "DB_URL=postgresql://") {
			t.Error(".env: DB_URL must be non-empty for pgsql")
		}
		if !strings.Contains(example, "DB_URL=\n") {
			t.Error(".env.example: DB_URL must be present but empty (redacted)")
		}
	}

	// .env and .env.example must expose the same set of keys
	envKeys := extractTestKeys(env)
	exampleKeys := extractTestKeys(example)
	envKeySet := make(map[string]bool, len(envKeys))
	for _, k := range envKeys {
		envKeySet[k] = true
	}
	for _, k := range exampleKeys {
		if !envKeySet[k] {
			t.Errorf(".env.example has key %q not present in .env", k)
		}
	}
	exKeySet := make(map[string]bool, len(exampleKeys))
	for _, k := range exampleKeys {
		exKeySet[k] = true
	}
	for _, k := range envKeys {
		if !exKeySet[k] {
			t.Errorf(".env has key %q not present in .env.example", k)
		}
	}

	// Dockerfile must embed the configured PHP version
	dockerfile := readTestFile(t, dir, "Dockerfile")
	if !strings.Contains(dockerfile, fx.cfg.PHP.Version) {
		t.Errorf("Dockerfile must reference PHP version %s", fx.cfg.PHP.Version)
	}
}

func readTestFile(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Errorf("readTestFile: %s: %v", name, err)
		return ""
	}
	return string(data)
}

// extractTestKeys returns all active (non-comment, non-disabled) key names from a .env string.
// Note: internal/compose has an identical extractKeys() helper; this duplicate exists because
// the two packages cannot share test helpers in Go.
func extractTestKeys(env string) []string {
	var keys []string
	for line := range strings.SplitSeq(env, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.IndexByte(line, '='); idx > 0 {
			keys = append(keys, line[:idx])
		}
	}
	return keys
}
