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
	if !cfg.Workers.Schedule {
		t.Error("Workers.Schedule should default to true")
	}
	if len(cfg.Workers.Queue) != 1 {
		t.Fatalf("Workers.Queue len = %d, want 1", len(cfg.Workers.Queue))
	}
	if cfg.Workers.Queue[0].Name != "default" {
		t.Errorf("Workers.Queue[0].Name = %q, want default", cfg.Workers.Queue[0].Name)
	}
	if cfg.Workers.Queue[0].Count != 1 {
		t.Errorf("Workers.Queue[0].Count = %d, want 1", cfg.Workers.Queue[0].Count)
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
  version: "12.*"
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
	if cfg.Laravel.Version != "12.*" {
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

func TestValidationBadLaravelVersion(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "version: 1\nlaravel:\n  version: \"11.*\"\n")
	_, err := Load(dir)
	if err == nil {
		t.Error("expected error for unsupported Laravel version")
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

func TestWorkersValid(t *testing.T) {
	cases := map[string]string{
		"schedule only": `
version: 1
workers:
  schedule: true
`,
		"queue only": `
version: 1
workers:
  queue:
    - queues: [default]
      count: 2
`,
		"both": `
version: 1
workers:
  schedule: true
  queue:
    - queues: [default]
      count: 1
`,
		"empty block": `
version: 1
workers: {}
`,
		"explicit name": `
version: 1
workers:
  queue:
    - name: fast
      queues: [high, default]
      count: 3
`,
		"derived name from queues": `
version: 1
workers:
  queue:
    - queues: [mail]
      count: 1
`,
		"derived name default fallback": `
version: 1
workers:
  queue:
    - count: 1
`,
		"multiple pools": `
version: 1
workers:
  queue:
    - name: high
      queues: [high]
      count: 2
    - name: low
      queues: [low]
      count: 1
`,
	}
	for name, yamlBody := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeYAML(t, dir, yamlBody)
			if _, err := Load(dir); err != nil {
				t.Fatalf("Load error: %v", err)
			}
		})
	}
}

func TestWorkersDefaultingDerivesName(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `
version: 1
workers:
  queue:
    - queues: [mail]
      count: 2
    - count: 1
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if got := cfg.Workers.Queue[0].Name; got != "mail" {
		t.Errorf("pool[0].Name = %q, want mail", got)
	}
	if got := cfg.Workers.Queue[1].Name; got != "default" {
		t.Errorf("pool[1].Name = %q, want default (derived from queues default)", got)
	}
	if got := cfg.Workers.Queue[1].Queues; len(got) != 1 || got[0] != "default" {
		t.Errorf("pool[1].Queues = %v, want [default]", got)
	}
}

func TestWorkersCountDefaultsToOne(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `
version: 1
workers:
  queue:
    - queues: [default]
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if got := cfg.Workers.Queue[0].Count; got != 1 {
		t.Errorf("Count = %d, want 1", got)
	}
}

func TestWorkersInvalid(t *testing.T) {
	cases := map[string]string{
		"duplicate pool names": `
version: 1
workers:
  queue:
    - name: default
      count: 1
    - queues: [default]
      count: 1
`,
		"bad name chars": `
version: 1
workers:
  queue:
    - name: Bad Name!
      count: 1
`,
		"uppercase name": `
version: 1
workers:
  queue:
    - name: HighPriority
      count: 1
`,
		"empty queues explicit": `
version: 1
workers:
  queue:
    - name: x
      queues: []
      count: 1
`,
		"negative tries": `
version: 1
workers:
  queue:
    - queues: [default]
      count: 1
      tries: -1
`,
		"negative timeout": `
version: 1
workers:
  queue:
    - queues: [default]
      count: 1
      timeout: -5
`,
	}
	for name, yamlBody := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeYAML(t, dir, yamlBody)
			if _, err := Load(dir); err == nil {
				t.Errorf("expected error for %s", name)
			}
		})
	}
}

func TestWorkersUnknownKeyWarning(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `
version: 1
workers:
  schedule: true
  futureThing: yes
  queue:
    - queues: [default]
      count: 1
      unknownField: foo
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
	out := string(buf[:n])
	if !containsAll(out, "futureThing", "unknownField") {
		t.Errorf("expected warnings for unknown keys, got: %q", out)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		found := false
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func TestPackageManagerDefault(t *testing.T) {
	cfg := New()
	if cfg.Node.PackageManager != DefaultPackageManager {
		t.Errorf("Node.PackageManager = %q, want %q", cfg.Node.PackageManager, DefaultPackageManager)
	}
}

func TestPackageManagerValid(t *testing.T) {
	for _, pm := range []string{"npm", "pnpm", "bun"} {
		t.Run(pm, func(t *testing.T) {
			dir := t.TempDir()
			writeYAML(t, dir, "version: 1\nnode:\n  packageManager: "+pm+"\n")
			cfg, err := Load(dir)
			if err != nil {
				t.Fatalf("Load error: %v", err)
			}
			if cfg.Node.PackageManager != pm {
				t.Errorf("Node.PackageManager = %q, want %q", cfg.Node.PackageManager, pm)
			}
		})
	}
}

func TestPackageManagerInvalid(t *testing.T) {
	for _, pm := range []string{"yarn", "bogus"} {
		t.Run(pm, func(t *testing.T) {
			dir := t.TempDir()
			writeYAML(t, dir, "version: 1\nnode:\n  packageManager: "+pm+"\n")
			if _, err := Load(dir); err == nil {
				t.Errorf("expected error for package manager %q", pm)
			}
		})
	}
}

func TestNodeUnknownKeyWarning(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `
version: 1
node:
  packageManager: pnpm
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
	out := string(buf[:n])
	if !containsAll(out, "futureThing") {
		t.Errorf("expected warning for unknown node key, got: %q", out)
	}
}

func TestServerDefaults(t *testing.T) {
	cfg := New()
	if !cfg.Server.IsHTTPS() {
		t.Error("Server.IsHTTPS() should default to true")
	}
	if got := cfg.Server.EffectivePort(); got != 443 {
		t.Errorf("Server.EffectivePort() = %d, want 443", got)
	}
}

func TestServerHTTPSExplicitFalse(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `
version: 1
server:
  https: false
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.Server.IsHTTPS() {
		t.Error("Server.IsHTTPS() should be false when explicitly set")
	}
	if got := cfg.Server.EffectivePort(); got != 80 {
		t.Errorf("Server.EffectivePort() = %d, want 80", got)
	}
}

func TestServerHTTPSWithCustomPort(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `
version: 1
server:
  https: true
  port: 4433
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if !cfg.Server.IsHTTPS() {
		t.Error("Server.IsHTTPS() should be true")
	}
	if got := cfg.Server.EffectivePort(); got != 4433 {
		t.Errorf("Server.EffectivePort() = %d, want 4433", got)
	}
}

func TestServerPortValidation(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `
version: 1
server:
  port: 99999
`)
	_, err := Load(dir)
	if err == nil {
		t.Error("expected error for invalid port 99999")
	}
}

func TestServerUnknownKeyWarning(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `
version: 1
server:
  https: true
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
	out := string(buf[:n])
	if !containsAll(out, "futureThing") {
		t.Errorf("expected warning for unknown server key, got: %q", out)
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

func TestAlias_UnmarshalYAML_String(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `
version: 1
aliases:
  lint: "vendor/bin/pint"
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	a, ok := cfg.Aliases["lint"]
	if !ok {
		t.Fatal("alias 'lint' not found")
	}
	if a.Cmd != "vendor/bin/pint" {
		t.Errorf("Cmd = %q, want %q", a.Cmd, "vendor/bin/pint")
	}
	if a.Host {
		t.Error("Host should be false for string shorthand")
	}
}

func TestAlias_UnmarshalYAML_Map(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `
version: 1
aliases:
  code:
    cmd: "code ."
    host: true
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	a, ok := cfg.Aliases["code"]
	if !ok {
		t.Fatal("alias 'code' not found")
	}
	if a.Cmd != "code ." {
		t.Errorf("Cmd = %q, want %q", a.Cmd, "code .")
	}
	if !a.Host {
		t.Error("Host should be true for map form with host: true")
	}
}

func TestValidateAliases_BuiltinCollision(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `
version: 1
aliases:
  artisan: "php artisan"
`)
	_, err := Load(dir)
	if err == nil {
		t.Error("expected error for builtin collision with 'artisan'")
	}
}

func TestValidateAliases_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `
version: 1
aliases:
  Artisan: "php artisan"
`)
	_, err := Load(dir)
	if err == nil {
		t.Error("expected error for case-insensitive collision with builtin 'artisan'")
	}
}

func TestValidateAliases_InvalidName(t *testing.T) {
	cases := map[string]string{
		"starts with digit": `
version: 1
aliases:
  1bad: "echo hi"
`,
		"special chars": `
version: 1
aliases:
  "no spaces": "echo hi"
`,
		"dot in name": `
version: 1
aliases:
  "bad.name": "echo hi"
`,
	}
	for name, yamlBody := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeYAML(t, dir, yamlBody)
			if _, err := Load(dir); err == nil {
				t.Errorf("expected error for invalid alias name: %s", name)
			}
		})
	}
}

func TestValidateAliases_EmptyCmd(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `
version: 1
aliases:
  lint:
    cmd: ""
`)
	_, err := Load(dir)
	if err == nil {
		t.Error("expected error for empty cmd")
	}
}

func TestValidateAliases_Valid(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `
version: 1
aliases:
  lint: "vendor/bin/pint"
  analyse:
    cmd: "vendor/bin/phpstan analyse"
  open-browser:
    cmd: "open http://localhost"
    host: true
  _internal: "some-cmd"
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if len(cfg.Aliases) != 4 {
		t.Errorf("Aliases count = %d, want 4", len(cfg.Aliases))
	}
}
