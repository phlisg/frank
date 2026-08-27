package worktreelist

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSeedMarkerLifecycle(t *testing.T) {
	wt := t.TempDir()

	if SeedPending(wt) {
		t.Fatal("fresh worktree should have no pending seed")
	}

	if err := markSeedDB(wt, "/main/repo"); err != nil {
		t.Fatalf("markSeedDB: %v", err)
	}

	if !SeedPending(wt) {
		t.Fatal("marker written but SeedPending is false")
	}

	data, err := os.ReadFile(filepath.Join(wt, ".frank", ".seed-db"))
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}

	if strings.TrimSpace(string(data)) != "/main/repo" {
		t.Errorf("marker holds %q, want /main/repo", data)
	}

	// No frank.yaml -> seedDB must not blow up; it just reports the failure.
	if err := seedDB(wt, os.Stderr); err == nil {
		t.Error("seedDB with no config should error")
	}
}

func TestDumpCommandsPassCredentialsViaEnv(t *testing.T) {
	src := map[string]string{"DB_USERNAME": "sail", "DB_PASSWORD": "srcpw", "DB_DATABASE": "app"}
	dst := map[string]string{"DB_USERNAME": "sail", "DB_PASSWORD": "dstpw", "DB_DATABASE": "app"}

	dump, restore := dumpCommands("pgsql", src, dst)
	if dump[0] != "env" || dump[1] != "PGPASSWORD=srcpw" || dump[2] != "pg_dump" {
		t.Errorf("pgsql dump = %v", dump)
	}

	if restore[1] != "PGPASSWORD=dstpw" || restore[2] != "psql" {
		t.Errorf("pgsql restore = %v", restore)
	}

	dump, restore = dumpCommands("mysql", src, dst)
	if dump[1] != "MYSQL_PWD=srcpw" || dump[2] != "mysqldump" {
		t.Errorf("mysql dump = %v", dump)
	}

	if restore[1] != "MYSQL_PWD=dstpw" || restore[2] != "mysql" {
		t.Errorf("mysql restore = %v", restore)
	}
}

func TestStartGuardsPerWorktreeNotGlobally(t *testing.T) {
	m := New(nil, t.TempDir())

	a := WorktreeItem{Path: "/wt/a", Branch: "feature/a"}
	b := WorktreeItem{Path: "/wt/b", Branch: "feature/b"}

	if !m.start(a) {
		t.Fatal("first action on a should start")
	}

	if m.start(a) {
		t.Error("second action on the same worktree must be refused")
	}

	if !m.start(b) {
		t.Error("a different worktree must be allowed to run in parallel")
	}
}

// Exercises the real git invocation — a bad flag is invisible to a mocked test.
func TestCreateWorktreeAgainstRealRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	// Under a git hook (lefthook pre-commit), git exports GIT_DIR/GIT_INDEX_FILE
	// as paths relative to the hooked repo. They would be inherited by every git
	// process this test spawns in its temp repo and resolve to nonsense there.
	for _, k := range []string{"GIT_DIR", "GIT_INDEX_FILE", "GIT_WORK_TREE"} {
		if v, ok := os.LookupEnv(k); ok {
			os.Unsetenv(k)
			t.Cleanup(func() { os.Setenv(k, v) })
		}
	}

	root := t.TempDir()
	repo := filepath.Join(root, "main")

	if err := os.MkdirAll(repo, 0755); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "t@example.com"},
		{"config", "user.name", "t"},
		{"commit", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo

		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}

	if err := os.WriteFile(filepath.Join(repo, ".env"), []byte("APP_KEY=base64:main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var log bytes.Buffer

	wt := filepath.Join(root, "main-feature-x")
	if err := CreateWorktree(repo, wt, "feature/x", true, &log); err != nil {
		t.Fatalf("CreateWorktree: %v\n%s", err, log.String())
	}

	if !SeedPending(wt) {
		t.Error("seed marker missing")
	}

	// .env must be inherited, or the worktree loses every project-specific key
	// and mints an APP_KEY that cannot decrypt cloned data.
	data, err := os.ReadFile(filepath.Join(wt, ".env"))
	if err != nil {
		t.Fatalf("worktree .env: %v", err)
	}

	if !strings.Contains(string(data), "APP_KEY=base64:main") {
		t.Errorf(".env not copied from main: %q", data)
	}
}
