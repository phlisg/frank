package watch

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
	ignore "github.com/sabhiram/go-gitignore"
)

// compileIgnore constructs the unified matcher: baseline patterns unioned with
// the project .gitignore. If the project .gitignore is missing, baseline
// alone applies. If the file exists but cannot be read, we log a WARN and
// fall back to baseline-only — the watcher must not crash (spec "parser
// errors … don't crash watcher").
//
// Nested .gitignore files (e.g. storage/.gitignore) are out of scope for v1;
// only the project-root .gitignore is consulted.
func compileIgnore(projectRoot string) *ignore.GitIgnore {
	baseline := append([]string(nil), baselineIgnorePatterns...)

	giPath := filepath.Join(projectRoot, ".gitignore")
	data, err := os.ReadFile(giPath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "frank watch: WARN failed to read %s (%v); falling back to baseline ignore patterns only\n", giPath, err)
		}
		return ignore.CompileIgnoreLines(baseline...)
	}

	lines := strings.Split(string(data), "\n")
	return ignore.CompileIgnoreLines(append(baseline, lines...)...)
}

// ignoreMatcher wraps the compiled GitIgnore with an interface that takes
// paths relative to the project root (forward-slash separated). Anchored
// patterns like `/storage` rely on this — the walker must never feed paths
// relative to an individual watch root or anchoring semantics break.
type ignoreMatcher struct {
	gi *ignore.GitIgnore
}

// Matches reports whether the given project-root-relative path should be
// ignored. For directories, both `foo` and `foo/` forms are checked since
// gitignore semantics treat trailing-slash patterns as directory-only.
func (m *ignoreMatcher) Matches(relPath string, isDir bool) bool {
	if m == nil || m.gi == nil {
		return false
	}
	p := filepath.ToSlash(relPath)
	if p == "" || p == "." {
		return false
	}
	if m.gi.MatchesPath(p) {
		return true
	}
	if isDir && !strings.HasSuffix(p, "/") {
		if m.gi.MatchesPath(p + "/") {
			return true
		}
	}
	return false
}

// armWatches walks the default watch roots (pruning ignored dirs before
// adding them) and arms inotify watches on each surviving directory. It
// also adds parent-dir watches for every file in defaultWatchFiles exactly
// once (events for the file are matched by basename at dispatch time).
//
// Returns the number of directories actually added to fsnotify. The first
// non-skip error from WalkDir (if any) is returned alongside; partial
// success is still returned so the caller can decide whether to proceed.
func (w *Watcher) armWatches() (int, error) {
	if w.fsw == nil {
		return 0, errors.New("watch: fsnotify watcher not initialized")
	}

	matcher := &ignoreMatcher{gi: w.gitignore}
	added := make(map[string]struct{})
	var firstErr error

	add := func(dir string) {
		if _, ok := added[dir]; ok {
			return
		}
		if err := w.fsw.Add(dir); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("add watch %q: %w", dir, err)
			}
			return
		}
		added[dir] = struct{}{}
	}

	for _, root := range defaultWatchRoots {
		absRoot := filepath.Join(w.cfg.ProjectRoot, root)
		info, err := os.Stat(absRoot)
		if err != nil || !info.IsDir() {
			// Missing root is not an error — Laravel projects vary; e.g.
			// `lang/` only exists on certain versions.
			continue
		}

		werr := filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				// Permission or transient read error — skip this subtree,
				// record the error for the caller but don't abort the walk.
				if firstErr == nil {
					firstErr = err
				}
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if !d.IsDir() {
				return nil
			}

			rel, relErr := filepath.Rel(w.cfg.ProjectRoot, path)
			if relErr != nil {
				return nil
			}

			if matcher.Matches(rel, true) {
				return fs.SkipDir
			}
			add(path)
			return nil
		})
		if werr != nil && firstErr == nil {
			firstErr = werr
		}
	}

	// defaultWatchFiles: watch each file's parent directory exactly once.
	// The classifier filters by basename at event time. Skip files whose
	// parent dir is ignored (defensive — unlikely for root-level files).
	for _, file := range defaultWatchFiles {
		parent := filepath.Dir(filepath.Join(w.cfg.ProjectRoot, file))
		info, err := os.Stat(parent)
		if err != nil || !info.IsDir() {
			continue
		}
		rel, relErr := filepath.Rel(w.cfg.ProjectRoot, parent)
		if relErr == nil && matcher.Matches(rel, true) {
			continue
		}
		add(parent)
	}

	fmt.Fprintf(os.Stderr, "frank watch: armed %d watches (roots=%d, files=%d, gitignore=%t)\n",
		len(added), len(defaultWatchRoots), len(defaultWatchFiles), w.gitignore != nil && hasGitignore(w.cfg.ProjectRoot))

	return len(added), firstErr
}

// handleDirEvent reacts to Create and Remove events for directories.
// Create: if a new directory appears under a watched root and isn't ignored,
// add an fsnotify watch so files created inside it are observed.
// Remove: remove the watch to avoid stale inotify descriptors.
func (w *Watcher) handleDirEvent(ev fsnotify.Event) {
	if w.fsw == nil {
		return
	}

	switch {
	case ev.Op&fsnotify.Create != 0:
		info, err := os.Lstat(ev.Name)
		if err != nil || !info.IsDir() {
			return
		}
		rel, err := filepath.Rel(w.cfg.ProjectRoot, ev.Name)
		if err != nil {
			return
		}
		matcher := &ignoreMatcher{gi: w.gitignore}
		if matcher.Matches(rel, true) {
			return
		}
		_ = w.fsw.Add(ev.Name)

	case ev.Op&fsnotify.Remove != 0:
		_ = w.fsw.Remove(ev.Name)
	}
}

// hasGitignore reports whether the project has a .gitignore file. Used only
// for the arm-time log — actual matcher construction tolerates absence.
func hasGitignore(projectRoot string) bool {
	_, err := os.Stat(filepath.Join(projectRoot, ".gitignore"))
	return err == nil
}

// classify decides whether a fsnotify event should trigger a reload.
//
// Trigger rules (spec "Event filter"):
//   - any .php file under a watched dir → trigger
//   - basename == ".env" → trigger
//   - basename == "composer.lock" → trigger
//   - anything matching the ignore set → no trigger
//   - everything else → no trigger
//
// Events for transient editor tempfiles (*.swp, *.swx, *~, 4913) are
// filtered by the ignore matcher, so they're handled alongside the
// gitignore patterns rather than special-cased here.
//
// The returned TriggerKind is advisory — in practice a triggering event
// fans out to BOTH queue:restart and schedule:restart (subject to
// ScheduleEnabled). The dispatcher (TODO: td-057aa5 consumes the event
// channel; td-a000b6 owns the fan-out) does that coordination.
func (w *Watcher) classify(event fsnotify.Event) (TriggerKind, bool) {
	// Only Create/Write/Rename/Remove influence reloads. Chmod alone is
	// ignored — noisy and not code-relevant.
	const wantOps = fsnotify.Create | fsnotify.Write | fsnotify.Rename | fsnotify.Remove
	if event.Op&wantOps == 0 {
		return TriggerQueueRestart, false
	}

	name := event.Name
	base := filepath.Base(name)

	// Defensive ignore check — even though arm-time pruning means ignored
	// dirs don't get inotify watches, an event could still fire for an
	// ignored file inside a watched dir (e.g. vim .swp written next to a
	// .php file in app/).
	if w.gitignore != nil {
		if rel, err := filepath.Rel(w.cfg.ProjectRoot, name); err == nil {
			m := &ignoreMatcher{gi: w.gitignore}
			if m.Matches(rel, false) {
				return TriggerQueueRestart, false
			}
		}
	}

	// Exact-basename matches for root-level single files.
	if base == ".env" || base == "composer.lock" {
		return TriggerQueueRestart, true
	}

	// .php extension — case-sensitive. Exclude obvious editor tempfiles
	// that escape the ignore set (e.g. "foo.php.swp" — Matches should
	// catch it via *.swp, but double-check via extension too).
	if filepath.Ext(base) == ".php" {
		return TriggerQueueRestart, true
	}

	return TriggerQueueRestart, false
}
