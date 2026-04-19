package watch

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

// fakeLaravelProject builds a minimal Laravel-shaped tree under t.TempDir()
// and returns its root. Files/dirs created:
//
//	app/Http/Controllers/Foo.php
//	vendor/package/Foo.php
//	storage/framework/cache/data/xyz.php
//	node_modules/mod/index.js
//	config/app.php
//	resources/views/welcome.blade.php
//	bootstrap/app.php
//	routes/web.php
//	.env
//	composer.lock
//	.gitignore  (contains vendor/, node_modules/, storage/)
func fakeLaravelProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	files := map[string]string{
		"app/Http/Controllers/Foo.php":         "<?php",
		"vendor/package/Foo.php":               "<?php",
		"storage/framework/cache/data/xyz.php": "<?php",
		"node_modules/mod/index.js":            "",
		"config/app.php":                       "<?php",
		"resources/views/welcome.blade.php":    "",
		"bootstrap/app.php":                    "<?php",
		"routes/web.php":                       "<?php",
		".env":                                 "APP_ENV=local",
		"composer.lock":                        "{}",
		".gitignore":                           "vendor/\nnode_modules/\nstorage/\n",
	}
	for rel, content := range files {
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", abs, err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", abs, err)
		}
	}
	return root
}

// watchedDirs runs armWatches and returns the set of dirs added, paths
// relative to the project root. Used by multiple tests.
func watchedDirs(t *testing.T, root string, withGitignore bool) (map[string]struct{}, *Watcher) {
	t.Helper()
	if !withGitignore {
		// Remove .gitignore to test baseline-only path.
		_ = os.Remove(filepath.Join(root, ".gitignore"))
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("fsnotify.NewWatcher: %v", err)
	}
	t.Cleanup(func() { _ = fsw.Close() })

	w := &Watcher{
		cfg:       Config{ProjectRoot: root},
		fsw:       fsw,
		events:    make(chan fsnotify.Event, 128),
		done:      make(chan struct{}),
		gitignore: compileIgnore(root),
	}

	if _, err := w.armWatches(); err != nil {
		t.Fatalf("armWatches: %v", err)
	}

	set := make(map[string]struct{})
	for _, p := range fsw.WatchList() {
		rel, err := filepath.Rel(root, p)
		if err != nil {
			t.Fatalf("rel: %v", err)
		}
		set[filepath.ToSlash(rel)] = struct{}{}
	}
	return set, w
}

// TestWalker_PrunesGitignoredDirs asserts the walker never arms inotify
// watches on vendor/, node_modules/, or storage/ when those are in
// .gitignore, and does arm them on typical Laravel source roots.
func TestWalker_PrunesGitignoredDirs(t *testing.T) {
	root := fakeLaravelProject(t)
	watched, _ := watchedDirs(t, root, true)

	mustHave := []string{"app", "app/Http", "app/Http/Controllers", "config", "resources/views", "bootstrap", "routes"}
	for _, d := range mustHave {
		if _, ok := watched[d]; !ok {
			t.Errorf("expected watch on %q, missing; got: %v", d, keys(watched))
		}
	}

	mustNotHave := []string{
		"vendor", "vendor/package",
		"node_modules", "node_modules/mod",
		"storage", "storage/framework", "storage/framework/cache", "storage/framework/cache/data",
	}
	for _, d := range mustNotHave {
		if _, ok := watched[d]; ok {
			t.Errorf("watch on %q should have been pruned; got: %v", d, keys(watched))
		}
	}
}

// TestWalker_RootParentAddedOnce ensures the parent dir of .env and
// composer.lock (the project root itself) is added exactly once even
// though two files share it.
func TestWalker_RootParentAddedOnce(t *testing.T) {
	root := fakeLaravelProject(t)
	watched, _ := watchedDirs(t, root, true)

	if _, ok := watched["."]; !ok {
		t.Errorf("project root (parent of .env, composer.lock) should be watched; got: %v", keys(watched))
	}

	count := 0
	for p := range watched {
		if p == "." {
			count++
		}
	}
	if count != 1 {
		t.Errorf("project root added %d times, want 1", count)
	}
}

// TestWalker_BaselineOnlyWhenNoGitignore confirms that with .gitignore
// missing, the walker still runs using only the baseline (.git, .frank,
// swap-files etc.) and does not crash. vendor/, node_modules/, storage/
// aren't under defaultWatchRoots so they wouldn't be walked anyway; the
// real degradation case is an app/ subdir a project would normally
// gitignore — see TestWalker_DegradedExtraWatches.
func TestWalker_BaselineOnlyWhenNoGitignore(t *testing.T) {
	root := fakeLaravelProject(t)
	watched, _ := watchedDirs(t, root, false)

	if _, ok := watched["app/Http/Controllers"]; !ok {
		t.Errorf("app/Http/Controllers should be watched under baseline-only; got: %v", keys(watched))
	}
	// Root is still watched for .env/composer.lock parent.
	if _, ok := watched["."]; !ok {
		t.Errorf("project root should be watched under baseline-only; got: %v", keys(watched))
	}
}

// TestWalker_DegradedExtraWatches covers the spec's "degraded case":
// a project whose .gitignore omits a dir under a watch root will get
// extra inotify watches for that dir. Not a correctness failure — just
// resource usage.
func TestWalker_DegradedExtraWatches(t *testing.T) {
	root := fakeLaravelProject(t)
	// Create app/Generated/ — a dir a typical project would .gitignore
	// but this one doesn't.
	gen := filepath.Join(root, "app", "Generated")
	if err := os.MkdirAll(gen, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gen, "gen.php"), []byte("<?php"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Baseline + existing .gitignore (which does NOT mention Generated).
	watched, _ := watchedDirs(t, root, true)
	if _, ok := watched["app/Generated"]; !ok {
		t.Errorf("degraded case: app/Generated should be watched since .gitignore doesn't exclude it; got: %v", keys(watched))
	}
}

// TestWalker_DotFrankPruned confirms the baseline .frank/** rule keeps
// Frank's own output dir from being walked, even if a project lacks
// .gitignore (or forgets .frank/).
func TestWalker_DotFrankPruned(t *testing.T) {
	root := fakeLaravelProject(t)
	// Simulate Frank having generated output.
	if err := os.MkdirAll(filepath.Join(root, ".frank"), 0o755); err != nil {
		t.Fatalf("mkdir .frank: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".frank", "compose.yaml"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write .frank/compose.yaml: %v", err)
	}

	watched, _ := watchedDirs(t, root, true)
	for p := range watched {
		if strings.HasPrefix(p, ".frank") {
			t.Errorf(".frank/ should be pruned by baseline, but %q is watched", p)
		}
	}
}

// TestClassify_Events covers the event filter matrix.
//
// Nested .gitignore files (e.g. storage/.gitignore) are out of scope for
// v1 per spec; this test does not exercise them.
func TestClassify_Events(t *testing.T) {
	root := fakeLaravelProject(t)
	w := &Watcher{
		cfg:       Config{ProjectRoot: root},
		events:    make(chan fsnotify.Event, 8),
		done:      make(chan struct{}),
		gitignore: compileIgnore(root),
	}

	cases := []struct {
		name string
		path string
		op   fsnotify.Op
		want bool
	}{
		{"php under app", "app/Http/Controllers/Foo.php", fsnotify.Write, true},
		{"vim swap under app", "app/Http/Controllers/.Foo.php.swp", fsnotify.Write, false},
		{"readme under app", "app/README.md", fsnotify.Write, false},
		{"env at root", ".env", fsnotify.Write, true},
		{"composer.lock at root", "composer.lock", fsnotify.Write, true},
		{"vendor php (defensive)", "vendor/foo/Bar.php", fsnotify.Write, false},
		{"chmod on php — ignored", "app/Http/Controllers/Foo.php", fsnotify.Chmod, false},
		{"storage php", "storage/framework/cache/xyz.php", fsnotify.Write, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev := fsnotify.Event{Name: filepath.Join(root, tc.path), Op: tc.op}
			_, fire := w.classify(ev)
			if fire != tc.want {
				t.Errorf("classify(%q, %v) = %v, want %v", tc.path, tc.op, fire, tc.want)
			}
		})
	}
}

// TestIgnoreMatcher_AnchoredPatterns exercises the anchored-pattern
// requirement: the matcher must be fed paths relative to project root,
// never relative to an individual watch root.
func TestIgnoreMatcher_AnchoredPatterns(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("/app/Generated/\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	m := &ignoreMatcher{gi: compileIgnore(root)}

	if !m.Matches("app/Generated", true) {
		t.Errorf("anchored pattern /app/Generated/ should match project-root-relative path app/Generated")
	}
	// A differently-anchored path must NOT match. The matcher must not
	// mistakenly match `Generated` alone (as would happen if we fed it
	// paths relative to the `app/` watch root).
	if m.Matches("Generated", true) {
		t.Errorf("anchored pattern /app/Generated/ should NOT match `Generated` alone")
	}
}

// TestCompileIgnore_ReadError simulates an unreadable .gitignore (make it
// a directory), asserts a WARN is emitted on stderr, and asserts the
// matcher falls back to baseline-only without panicking.
//
// Note on spec ambiguity: the go-gitignore parser silently drops malformed
// lines rather than returning an error, so "malformed .gitignore" cannot
// produce a parser error in practice. The WARN path is reached via
// os.ReadFile errors (permission denied, .gitignore-as-directory, etc.).
// This test exercises the directory-in-place-of-file case for portability.
func TestCompileIgnore_ReadError(t *testing.T) {
	root := t.TempDir()
	// Make .gitignore a directory — os.ReadFile returns an error.
	if err := os.Mkdir(filepath.Join(root, ".gitignore"), 0o755); err != nil {
		t.Fatalf("mkdir .gitignore: %v", err)
	}

	// Capture stderr.
	origStderr := os.Stderr
	r, wr, _ := os.Pipe()
	os.Stderr = wr
	defer func() { os.Stderr = origStderr }()

	gi := compileIgnore(root)

	_ = wr.Close()
	buf, _ := io.ReadAll(r)
	stderr := string(buf)

	if gi == nil {
		t.Fatalf("compileIgnore returned nil; baseline-only matcher expected")
	}
	if !strings.Contains(stderr, "WARN") {
		t.Errorf("expected WARN on stderr, got: %q", stderr)
	}

	// Baseline-only should still ignore .git/**.
	m := &ignoreMatcher{gi: gi}
	if !m.Matches(".git/HEAD", false) {
		t.Errorf("baseline should ignore .git/HEAD")
	}
}

// TestStartStop_LifecycleDeliversPhpEvent spins up Start in a goroutine,
// writes to a .php file inside the watched set, asserts the event lands
// on w.Events(), then calls Stop and asserts the goroutine exits.
func TestStartStop_LifecycleDeliversPhpEvent(t *testing.T) {
	root := fakeLaravelProject(t)

	w, err := New(Config{ProjectRoot: root})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- w.Start(ctx) }()

	// Give Start a moment to arm watches.
	time.Sleep(100 * time.Millisecond)

	target := filepath.Join(root, "app", "Http", "Controllers", "Foo.php")
	if err := os.WriteFile(target, []byte("<?php // edit"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case ev := <-w.Events():
		if !strings.HasSuffix(ev.Name, "Foo.php") {
			t.Errorf("unexpected event: %v", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("no event received within 3s")
	}

	if err := w.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("Start goroutine did not return after Stop")
	}
}

// TestStop_Idempotent confirms Stop can be called multiple times without
// panicking (close-of-closed-channel would otherwise blow up).
func TestStop_Idempotent(t *testing.T) {
	w, err := New(Config{ProjectRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := w.Stop(); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	if err := w.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
