package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phlisg/frank/internal/config"
)

// frank.yaml fixtures for autoRegenerate tests. HTTPS disabled to avoid mkcert
// side effects; keeps the rendered Dockerfile deterministic across machines.
const (
	yamlFrankenphpBase = `version: 1
php:
  version: "8.4"
  runtime: frankenphp
services:
  - pgsql
  - mailpit
server:
  https: false
`
	yamlFrankenphpWorkers = `version: 1
php:
  version: "8.4"
  runtime: frankenphp
services:
  - pgsql
  - mailpit
server:
  https: false
workers:
  queue:
    - name: default
      queues:
        - priority
        - default
        - low
      count: 2
`
	yamlFrankenphpAddRedis = `version: 1
php:
  version: "8.4"
  runtime: frankenphp
services:
  - pgsql
  - mailpit
  - redis
server:
  https: false
`
	yamlFrankenphpPHP85 = `version: 1
php:
  version: "8.5"
  runtime: frankenphp
services:
  - pgsql
  - mailpit
server:
  https: false
`
	yamlFPMBase = `version: 1
php:
  version: "8.4"
  runtime: fpm
services:
  - mysql
  - redis
server:
  https: false
`
)

// seedFrankProject writes frank.yaml into a fresh temp dir and runs generate so
// .frank/ (incl .state with configHash + the rendered Dockerfile) exists, as if
// the user had previously run `frank generate`. Returns the project dir.
func seedFrankProject(t *testing.T, yaml, version string) string {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "proj")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "frank.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatalf("write frank.yaml: %v", err)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("seed config.Load: %v", err)
	}

	if err := generate(cfg, dir, version); err != nil {
		t.Fatalf("seed generate: %v", err)
	}

	return dir
}

func writeYAML(t *testing.T, dir, yaml string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, "frank.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatalf("rewrite frank.yaml: %v", err)
	}
}

func TestAutoRegenerate(t *testing.T) {
	tests := []struct {
		name string
		// mutate runs after seeding and before autoRegenerate. It returns the
		// version to pass to autoRegenerate (seed always stamps "1.0.0").
		seedYAML  string
		mutate    func(t *testing.T, dir string) (version string)
		wantRegen bool
		wantBuild bool
	}{
		{
			name:     "no change",
			seedYAML: yamlFrankenphpBase,
			mutate:   func(t *testing.T, dir string) string { return "1.0.0" },
			// hash + version match → fast path, nothing happens.
			wantRegen: false,
			wantBuild: false,
		},
		{
			name:     "queue edit regenerates without build",
			seedYAML: yamlFrankenphpBase,
			mutate: func(t *testing.T, dir string) string {
				writeYAML(t, dir, yamlFrankenphpWorkers)
				return "1.0.0"
			},
			wantRegen: true,
			wantBuild: false, // workers don't touch the Dockerfile
		},
		{
			name:     "add service regenerates without build",
			seedYAML: yamlFrankenphpBase,
			mutate: func(t *testing.T, dir string) string {
				writeYAML(t, dir, yamlFrankenphpAddRedis)
				return "1.0.0"
			},
			wantRegen: true,
			wantBuild: false, // services are separate containers, not image inputs
		},
		{
			name:     "php version change forces build",
			seedYAML: yamlFrankenphpBase,
			mutate: func(t *testing.T, dir string) string {
				writeYAML(t, dir, yamlFrankenphpPHP85)
				return "1.0.0"
			},
			wantRegen: true,
			wantBuild: true, // Dockerfile embeds the PHP version
		},
		{
			name:     "runtime change forces build",
			seedYAML: yamlFrankenphpBase,
			mutate: func(t *testing.T, dir string) string {
				writeYAML(t, dir, yamlFPMBase)
				return "1.0.0"
			},
			wantRegen: true,
			wantBuild: true, // entirely different Dockerfile template
		},
		{
			name:      "version bump with identical dockerfile skips build",
			seedYAML:  yamlFrankenphpBase,
			mutate:    func(t *testing.T, dir string) string { return "1.1.0" },
			wantRegen: true,  // frank version bumped
			wantBuild: false, // but the rendered Dockerfile is unchanged
		},
		{
			name:     "state missing regenerates, dockerfile intact skips build",
			seedYAML: yamlFrankenphpBase,
			mutate: func(t *testing.T, dir string) string {
				os.Remove(filepath.Join(dir, ".frank", ".state"))
				return "1.0.0"
			},
			wantRegen: true,
			wantBuild: false, // on-disk Dockerfile still matches the re-render
		},
		{
			name:     "state corrupt regenerates, dockerfile intact skips build",
			seedYAML: yamlFrankenphpBase,
			mutate: func(t *testing.T, dir string) string {
				os.WriteFile(filepath.Join(dir, ".frank", ".state"), []byte("{not json"), 0644)
				return "1.0.0"
			},
			wantRegen: true,
			wantBuild: false,
		},
		{
			name:     "dev build with deleted dockerfile forces build",
			seedYAML: yamlFrankenphpBase,
			mutate: func(t *testing.T, dir string) string {
				os.Remove(filepath.Join(dir, ".frank", "Dockerfile"))
				return "dev" // dev fires Tier 1; deletion alone would not
			},
			wantRegen: true,
			wantBuild: true, // missing Dockerfile → fail safe toward rebuild
		},
		{
			name:     "fpm nginx dockerfile deletion forces build",
			seedYAML: yamlFPMBase,
			mutate: func(t *testing.T, dir string) string {
				// Prove nginx.Dockerfile is part of the diff set: delete only it,
				// leave the primary Dockerfile intact.
				os.Remove(filepath.Join(dir, ".frank", "nginx.Dockerfile"))
				return "dev"
			},
			wantRegen: true,
			wantBuild: true,
		},
		{
			name:     "missing base.Dockerfile forces structural regen + build",
			seedYAML: yamlFrankenphpBase,
			mutate: func(t *testing.T, dir string) string {
				// Pre-split project: monolithic Dockerfile present, base absent.
				// Version + hash both match, so only the structural trigger fires.
				os.Remove(filepath.Join(dir, ".frank", "base.Dockerfile"))
				return "1.0.0"
			},
			wantRegen: true,
			wantBuild: true, // base.Dockerfile missing → dockerfileChanged fail-safe
		},
		{
			name:     "edited base.Dockerfile forces build on regen",
			seedYAML: yamlFrankenphpBase,
			mutate: func(t *testing.T, dir string) string {
				// base.Dockerfile drifted from template; dev fires Tier 1 regen,
				// dockerfileChanged then sees the mismatch and forces --build.
				os.WriteFile(filepath.Join(dir, ".frank", "base.Dockerfile"), []byte("FROM scratch\n"), 0644)
				return "dev"
			},
			wantRegen: true,
			wantBuild: true,
		},
		{
			name:     "malformed yaml skips gracefully",
			seedYAML: yamlFrankenphpBase,
			mutate: func(t *testing.T, dir string) string {
				// Hash differs from stored → Tier 1 fires, but config.Load fails.
				writeYAML(t, dir, "php: [this is: not valid")
				return "1.0.0"
			},
			wantRegen: false, // graceful skip, normal up flow handles the error
			wantBuild: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := seedFrankProject(t, tt.seedYAML, "1.0.0")
			version := tt.mutate(t, dir)

			regen, build, err := autoRegenerate(dir, version)
			if err != nil {
				t.Fatalf("autoRegenerate: %v", err)
			}

			if regen != tt.wantRegen {
				t.Errorf("regenerated = %v, want %v", regen, tt.wantRegen)
			}

			if build != tt.wantBuild {
				t.Errorf("needsBuild = %v, want %v", build, tt.wantBuild)
			}
		})
	}
}

// TestAutoRegenerate_QueueRepro is the original bug: editing the queue list must
// regenerate compose with the new --queue CSV, without forcing an image rebuild.
func TestAutoRegenerate_QueueRepro(t *testing.T) {
	dir := seedFrankProject(t, yamlFrankenphpBase, "1.0.0")
	writeYAML(t, dir, yamlFrankenphpWorkers)

	regen, build, err := autoRegenerate(dir, "1.0.0")
	if err != nil {
		t.Fatalf("autoRegenerate: %v", err)
	}

	if !regen {
		t.Fatal("expected regeneration after queue edit")
	}

	if build {
		t.Error("queue edit must not force --build")
	}

	compose := readTestFile(t, dir, ".frank/compose.yaml")
	if !strings.Contains(compose, "priority,default,low") {
		t.Errorf("regenerated compose missing new queue CSV --queue=priority,default,low\n%s", compose)
	}
}

// TestDockerfileChanged_BaseDockerfile proves base.Dockerfile is part of the
// diff set: a freshly generated project reports no change, while deleting or
// editing base.Dockerfile (leaving the primary Dockerfile intact) flips it true.
func TestDockerfileChanged_BaseDockerfile(t *testing.T) {
	dir := seedFrankProject(t, yamlFrankenphpBase, "1.0.0")

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	if dockerfileChanged(dir, cfg) {
		t.Fatal("freshly generated project should report no dockerfile change")
	}

	// Edit base.Dockerfile only → must report changed.
	base := filepath.Join(dir, ".frank", "base.Dockerfile")
	if err := os.WriteFile(base, []byte("FROM scratch\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if !dockerfileChanged(dir, cfg) {
		t.Error("edited base.Dockerfile should report changed")
	}

	// Delete base.Dockerfile → must report changed.
	if err := os.Remove(base); err != nil {
		t.Fatal(err)
	}

	if !dockerfileChanged(dir, cfg) {
		t.Error("missing base.Dockerfile should report changed")
	}
}

// TestFrankConfigHash_Deterministic ensures the hash is stable across calls so
// it doesn't churn or cause spurious regenerations.
func TestFrankConfigHash_Deterministic(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "proj")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	writeYAML(t, dir, yamlFrankenphpBase)

	h1 := frankConfigHash(dir)
	h2 := frankConfigHash(dir)

	if h1 == "" {
		t.Fatal("hash empty for present frank.yaml")
	}

	if h1 != h2 {
		t.Errorf("hash not deterministic: %q != %q", h1, h2)
	}

	// A semantic change must flip the hash.
	writeYAML(t, dir, yamlFrankenphpPHP85)

	if h3 := frankConfigHash(dir); h3 == h1 {
		t.Error("hash unchanged after editing frank.yaml")
	}

	// Missing file → empty hash (treated as not-drifted by Tier 1).
	os.Remove(filepath.Join(dir, "frank.yaml"))

	if h := frankConfigHash(dir); h != "" {
		t.Errorf("missing frank.yaml should hash to empty, got %q", h)
	}
}
