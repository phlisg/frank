package cmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
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
		files: []string{".frank/compose.yaml", ".env", ".env.example", ".frank/Dockerfile", ".frank/Caddyfile", ".frank/vite-server.js", ".mcp.json"},
	},
	{
		name: "fpm-mysql-redis",
		cfg: &config.Config{
			PHP:      config.PHP{Version: "8.4", Runtime: "fpm"},
			Laravel:  config.Laravel{Version: "12.x"},
			Services: []string{"mysql", "redis"},
		},
		files: []string{".frank/compose.yaml", ".env", ".env.example", ".frank/Dockerfile", ".frank/nginx.conf", ".frank/nginx.Dockerfile", ".frank/vite-server.js", ".mcp.json"},
	},
	{
		name: "frankenphp-sqlite",
		cfg: &config.Config{
			PHP:      config.PHP{Version: "8.5", Runtime: "frankenphp"},
			Laravel:  config.Laravel{Version: "13.x"},
			Services: []string{"sqlite"},
		},
		files: []string{".frank/compose.yaml", ".env", ".env.example", ".frank/Dockerfile", ".frank/Caddyfile", ".frank/vite-server.js", ".mcp.json"},
	},
	{
		name: "frankenphp-pgsql-pnpm",
		cfg: &config.Config{
			PHP:      config.PHP{Version: "8.5", Runtime: "frankenphp"},
			Laravel:  config.Laravel{Version: "13.x"},
			Services: []string{"pgsql", "mailpit"},
			Node:     config.Node{PackageManager: "pnpm"},
		},
		files: []string{".frank/compose.yaml", ".env", ".env.example", ".frank/Dockerfile", ".frank/Caddyfile", ".frank/vite-server.js", ".mcp.json"},
	},
	{
		name: "frankenphp-pgsql-workers",
		cfg: &config.Config{
			PHP:      config.PHP{Version: "8.5", Runtime: "frankenphp"},
			Laravel:  config.Laravel{Version: "13.x"},
			Services: []string{"pgsql", "mailpit"},
			Workers: config.Workers{
				Schedule: true,
				Queue: []config.QueuePool{
					{Name: "default", Queues: []string{"default"}, Count: 2},
				},
			},
		},
		files: []string{".frank/compose.yaml", ".env", ".env.example", ".frank/Dockerfile", ".frank/Caddyfile", ".frank/vite-server.js", ".mcp.json"},
	},
	{
		name: "fpm-mysql-redis-workers",
		cfg: &config.Config{
			PHP:      config.PHP{Version: "8.4", Runtime: "fpm"},
			Laravel:  config.Laravel{Version: "12.x"},
			Services: []string{"mysql", "redis"},
			Workers: config.Workers{
				Schedule: false,
				Queue: []config.QueuePool{
					{Name: "high", Queues: []string{"high"}, Count: 1},
					{Name: "default", Queues: []string{"default"}, Count: 2},
				},
			},
		},
		files: []string{".frank/compose.yaml", ".env", ".env.example", ".frank/Dockerfile", ".frank/nginx.conf", ".frank/nginx.Dockerfile", ".frank/vite-server.js", ".mcp.json"},
	},
	{
		name: "frankenphp-pgsql-no-https",
		cfg: &config.Config{
			PHP:      config.PHP{Version: "8.5", Runtime: "frankenphp"},
			Laravel:  config.Laravel{Version: "13.x"},
			Services: []string{"pgsql", "mailpit"},
			Server:   config.Server{HTTPS: new(bool)},
		},
		files: []string{".frank/compose.yaml", ".env", ".env.example", ".frank/Dockerfile", ".frank/Caddyfile", ".frank/vite-server.js", ".mcp.json"},
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

			if err := generate(fx.cfg, dir, "dev"); err != nil {
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
					if err := os.MkdirAll(filepath.Dir(goldenPath), 0755); err != nil {
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
	dockerfile := readTestFile(t, dir, ".frank/Dockerfile")
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

// TestGenerate_BaseDockerfile verifies generate() emits both a thin
// .frank/Dockerfile (FROM frank/runtime:<tag>) and a self-contained
// .frank/base.Dockerfile (FROM dunglas/frankenphp, no frank/runtime ref).
// Uses the real generate() pipeline in a tempdir, matching the integration
// harness above.
func TestGenerate_BaseDockerfile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "frankenphp-base")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	cfg := &config.Config{
		PHP:      config.PHP{Version: "8.5", Runtime: "frankenphp"},
		Laravel:  config.Laravel{Version: "13.x"},
		Services: []string{"pgsql", "mailpit"},
	}
	if err := generate(cfg, dir, "dev"); err != nil {
		t.Fatalf("generate: %v", err)
	}

	base := readTestFile(t, dir, ".frank/base.Dockerfile")
	if !strings.Contains(base, "FROM dunglas/frankenphp") {
		t.Errorf("base.Dockerfile must build from upstream image, got:\n%s", base)
	}
	if strings.Contains(base, "FROM frank/runtime") {
		t.Error("base.Dockerfile must not reference frank/runtime (it IS the base)")
	}

	thin := readTestFile(t, dir, ".frank/Dockerfile")
	if !strings.Contains(thin, "FROM frank/runtime") {
		t.Errorf("thin Dockerfile must derive from frank/runtime, got:\n%s", thin)
	}
}

func TestWriteMCPConfig(t *testing.T) {
	t.Run("create_new", func(t *testing.T) {
		dir := t.TempDir()
		if err := writeMCPConfig(dir); err != nil {
			t.Fatalf("writeMCPConfig: %v", err)
		}
		root := readMCPJSON(t, dir)
		assertFrankServer(t, root)
	})

	t.Run("merge_existing", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, ".mcp.json", `{
  "mcpServers": {
    "other-server": {
      "command": "other",
      "args": ["serve"]
    }
  }
}`)
		if err := writeMCPConfig(dir); err != nil {
			t.Fatalf("writeMCPConfig: %v", err)
		}
		root := readMCPJSON(t, dir)
		assertFrankServer(t, root)

		servers := root["mcpServers"].(map[string]any)
		if _, ok := servers["other-server"]; !ok {
			t.Error("other-server entry was lost during merge")
		}
	})

	t.Run("overwrite_frank", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, ".mcp.json", `{
  "mcpServers": {
    "frank": {
      "command": "old-frank",
      "args": ["old"]
    }
  }
}`)
		if err := writeMCPConfig(dir); err != nil {
			t.Fatalf("writeMCPConfig: %v", err)
		}
		root := readMCPJSON(t, dir)
		assertFrankServer(t, root)
	})

	t.Run("malformed_json", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, ".mcp.json", "this is not json")
		if err := writeMCPConfig(dir); err != nil {
			t.Fatalf("writeMCPConfig: %v", err)
		}
		root := readMCPJSON(t, dir)
		assertFrankServer(t, root)
	})

	t.Run("preserves_other_top_level_keys", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, ".mcp.json", `{
  "mcpServers": {"existing": {"command": "x"}},
  "someOtherKey": "value"
}`)
		if err := writeMCPConfig(dir); err != nil {
			t.Fatalf("writeMCPConfig: %v", err)
		}
		root := readMCPJSON(t, dir)
		assertFrankServer(t, root)

		servers := root["mcpServers"].(map[string]any)
		if _, ok := servers["existing"]; !ok {
			t.Error("existing server entry was lost during merge")
		}
		if v, ok := root["someOtherKey"]; !ok || v != "value" {
			t.Errorf("someOtherKey lost or changed: got %v", v)
		}
	})
}

// readMCPJSON reads and parses .mcp.json from dir.
func readMCPJSON(t *testing.T, dir string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if err != nil {
		t.Fatalf("read .mcp.json: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse .mcp.json: %v\ncontent: %s", err, data)
	}
	return root
}

// assertFrankServer verifies the frank entry in mcpServers has the expected shape.
func assertFrankServer(t *testing.T, root map[string]any) {
	t.Helper()
	servers, ok := root["mcpServers"].(map[string]any)
	if !ok {
		t.Fatal("mcpServers key missing or not an object")
	}
	frank, ok := servers["frank"].(map[string]any)
	if !ok {
		t.Fatal("mcpServers.frank missing or not an object")
	}
	if frank["command"] != "frank" {
		t.Errorf("frank command: got %v, want frank", frank["command"])
	}
	args, ok := frank["args"].([]any)
	if !ok || len(args) != 1 || args[0] != "mcp" {
		t.Errorf("frank args: got %v, want [mcp]", frank["args"])
	}
}

// writeTestFile is a test helper that writes content to dir/name.
func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("writeFile %s: %v", name, err)
	}
}

func TestGenerate_WorktreeIntegration(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	cleanEnv := os.Environ()
	filtered := cleanEnv[:0]
	for _, e := range cleanEnv {
		if !strings.HasPrefix(e, "GIT_DIR=") && !strings.HasPrefix(e, "GIT_WORK_TREE=") && !strings.HasPrefix(e, "GIT_INDEX_FILE=") {
			filtered = append(filtered, e)
		}
	}

	git := func(dir string, args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = filtered
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s (%v)", args, out, err)
		}
	}

	mainDir := filepath.Join(t.TempDir(), "main-project")
	if err := os.MkdirAll(mainDir, 0755); err != nil {
		t.Fatal(err)
	}

	git(mainDir, "init")
	git(mainDir, "config", "user.email", "test@test.com")
	git(mainDir, "config", "user.name", "Test")
	os.WriteFile(filepath.Join(mainDir, "frank.yaml"), []byte("version: 1\n"), 0644)
	git(mainDir, "add", ".")
	git(mainDir, "commit", "-m", "init")

	wtDir := filepath.Join(t.TempDir(), "my-worktree")
	git(mainDir, "worktree", "add", wtDir)

	cfg := &config.Config{
		PHP:      config.PHP{Version: "8.5", Runtime: "frankenphp"},
		Laravel:  config.Laravel{Version: "latest"},
		Services: []string{"pgsql", "mailpit"},
	}

	// Generate in worktree — should get ephemeral ports.
	if err := generate(cfg, wtDir, "dev"); err != nil {
		t.Fatalf("generate in worktree: %v", err)
	}

	wtCompose := readTestFile(t, wtDir, ".frank/compose.yaml")
	wtVite := readTestFile(t, wtDir, ".frank/vite-server.js")

	// Worktree compose must have container-only ports (no host binding).
	if strings.Contains(wtCompose, "5432:5432") {
		t.Error("worktree compose should not have host-bound pgsql port")
	}
	if !strings.Contains(wtCompose, `"5432"`) {
		t.Error("worktree compose should have container-only pgsql port")
	}
	if strings.Contains(wtCompose, "1025:1025") {
		t.Error("worktree compose should not have host-bound mailpit port")
	}

	// Vite port should not be 5173.
	expectedVitePort := config.ViteWorktreePort("my-worktree")
	if !strings.Contains(wtCompose, fmt.Sprintf("%d:5173", expectedVitePort)) {
		t.Errorf("worktree compose should map vite to port %d", expectedVitePort)
	}
	if !strings.Contains(wtVite, fmt.Sprintf("localhost:%d", expectedVitePort)) {
		t.Errorf("worktree vite-server.js should reference port %d", expectedVitePort)
	}

	// Generate in main repo — should get normal host-bound ports.
	if err := generate(cfg, mainDir, "dev"); err != nil {
		t.Fatalf("generate in main: %v", err)
	}

	mainCompose := readTestFile(t, mainDir, ".frank/compose.yaml")
	mainVite := readTestFile(t, mainDir, ".frank/vite-server.js")

	if !strings.Contains(mainCompose, "5432:5432") {
		t.Error("main compose should have host-bound pgsql port")
	}
	if !strings.Contains(mainCompose, "5173:5173") {
		t.Error("main compose should have standard vite port")
	}
	if !strings.Contains(mainVite, "localhost:5173") {
		t.Error("main vite-server.js should reference port 5173")
	}
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
