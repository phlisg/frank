package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeYAML(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ConfigFileName), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestDefaults(t *testing.T) {
	cfg := New()
	if cfg.PHP.Version != DefaultPHPVersion {
		t.Errorf("PHP.Version = %q, want %q", cfg.PHP.Version, DefaultPHPVersion)
	}
	if cfg.PHP.Runtime != DefaultPHPRuntime {
		t.Errorf("PHP.Runtime = %q, want %q", cfg.PHP.Runtime, DefaultPHPRuntime)
	}
	if cfg.Laravel.Version != DefaultLaravelVersion {
		t.Errorf("Laravel.Version = %q, want %q", cfg.Laravel.Version, DefaultLaravelVersion)
	}
	if len(cfg.Services) != 2 || cfg.Services[0] != "pgsql" || cfg.Services[1] != "mailpit" {
		t.Errorf("Services = %v, want [pgsql mailpit]", cfg.Services)
	}
}

func TestLoadMinimal(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "version: 1\n")

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.PHP.Version != DefaultPHPVersion {
		t.Errorf("PHP.Version = %q after minimal load", cfg.PHP.Version)
	}
}

func TestLoadFull(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `
version: 1
php:
  version: "8.3"
  runtime: fpm
laravel:
  version: "11.*"
services:
  - mysql
  - redis
  - mailpit
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.PHP.Version != "8.3" {
		t.Errorf("PHP.Version = %q", cfg.PHP.Version)
	}
	if cfg.PHP.Runtime != "fpm" {
		t.Errorf("PHP.Runtime = %q", cfg.PHP.Runtime)
	}
	if cfg.Laravel.Version != "11.*" {
		t.Errorf("Laravel.Version = %q", cfg.Laravel.Version)
	}
	if !cfg.HasService("redis") {
		t.Error("expected redis service")
	}
	if cfg.Database() != "mysql" {
		t.Errorf("Database() = %q, want mysql", cfg.Database())
	}
}

func TestValidationBadPHPVersion(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "version: 1\nphp:\n  version: \"7.4\"\n")
	_, err := Load(dir)
	if err == nil {
		t.Error("expected error for unsupported PHP version")
	}
}

func TestValidationBadRuntime(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "version: 1\nphp:\n  runtime: apache\n")
	_, err := Load(dir)
	if err == nil {
		t.Error("expected error for unsupported runtime")
	}
}

func TestValidationBadService(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "version: 1\nservices:\n  - mongodb\n")
	_, err := Load(dir)
	if err == nil {
		t.Error("expected error for unsupported service")
	}
}

func TestValidationMultipleDatabases(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "version: 1\nservices:\n  - pgsql\n  - mysql\n")
	_, err := Load(dir)
	if err == nil {
		t.Error("expected error for multiple databases")
	}
}

func TestProjectName(t *testing.T) {
	name := ProjectName("/some/path/my-project")
	if name != "my-project" {
		t.Errorf("ProjectName = %q, want my-project", name)
	}
}

func TestHasService(t *testing.T) {
	cfg := &Config{Services: []string{"pgsql", "mailpit"}}
	if !cfg.HasService("pgsql") {
		t.Error("HasService(pgsql) = false")
	}
	if cfg.HasService("redis") {
		t.Error("HasService(redis) = true")
	}
}
