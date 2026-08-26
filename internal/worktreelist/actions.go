package worktreelist

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/docker"
)

func openBrowser(item WorktreeItem) error {
	if !item.IsRunning() {
		return fmt.Errorf("worktree not running")
	}

	port := item.WebPort()
	if port == 0 {
		return fmt.Errorf("no published port found for laravel.test")
	}

	cfg, err := config.Load(item.Path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	scheme := "http"
	if cfg.Server.IsHTTPS() {
		scheme = "https"
	}

	url := fmt.Sprintf("%s://localhost:%d", scheme, port)

	var opener string

	switch runtime.GOOS {
	case "darwin":
		opener = "open"
	default:
		opener = "xdg-open"
	}

	return exec.Command(opener, url).Start()
}

// RemoveWorktree deletes everything the worktree owns: ad-hoc workers, containers,
// named volumes, orphans and the images built for it. All of those are scoped to
// the worktree's compose project, so once its directory and branch are gone nothing
// can address them again and they are pure wasted disk. Deliberately far more
// destructive than `frank down`, which must preserve data.
func RemoveWorktree(path, branch string, w io.Writer) error {
	// Route the teardown through frank itself so ad-hoc workers — which live
	// outside compose.yaml and so survive a plain `compose down` — are cleaned up.
	_ = runFrank(w, "down", "--dir", path)

	_ = docker.New(path).RunStream(w, "down", "-v", "--remove-orphans", "--rmi", "local")

	// --rmi local skips services that name an image explicitly (the workers and the
	// vite sidecar all point at the app image), so remove the tags directly. Both
	// spellings exist depending on how the image was built.
	project := config.ProjectName(path)
	docker.RemoveImages(w, []string{
		fmt.Sprintf("frank-%s-laravel.test", project),
		fmt.Sprintf("%s-laravel.test", project),
	})

	out, err := exec.Command("git", "worktree", "remove", "--force", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree remove: %s", out)
	}

	if branch != "" && branch != "(detached)" {
		_ = exec.Command("git", "branch", "-D", branch).Run()
	}

	return nil
}

func needsGenerate(path string) bool {
	_, err := os.Stat(filepath.Join(path, ".frank", "compose.yaml"))
	return os.IsNotExist(err)
}

func upContainers(path string, w io.Writer) error {
	if needsGenerate(path) {
		if err := regenerate(path, w); err != nil {
			return err
		}
	}

	if err := runFrank(w, "up", "-d", "--dir", path); err != nil {
		return err
	}

	return seedDB(path, w)
}

func downContainers(path string, w io.Writer) error {
	return runFrank(w, "down", "--dir", path)
}

// runFrank shells out to this same binary, streaming its output to w (the TUI
// progress line) while keeping a copy for the error message. Piping the child's
// stdout means output.Region sees a non-TTY and emits plain tick lines instead
// of its ANSI-redrawing live region, which would trash the alt-screen.
func runFrank(w io.Writer, args ...string) error {
	frank, err := os.Executable()
	if err != nil {
		frank = "frank"
	}

	var buf bytes.Buffer

	cmd := exec.Command(frank, args...)
	cmd.Stdout = io.MultiWriter(w, &buf)
	cmd.Stderr = cmd.Stdout

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("frank %s: %s", args[0], strings.TrimSpace(buf.String()))
	}

	return nil
}

func openEditor(path string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		return fmt.Errorf("$EDITOR not set — export EDITOR to use this action")
	}

	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func tailLogs(path string) error {
	return docker.New(path).Run("logs", "-f", "--tail", "50")
}

// CreateWorktree adds a sibling worktree for branch. With seedDB set, the first
// `up` clones repoDir's database into it instead of starting from an empty one.
// Progress goes to w: checking out a large tree is slow enough that a silent
// wait reads as a hang.
func CreateWorktree(repoDir, wtPath, branch string, seedDB bool, w io.Writer) error {
	fmt.Fprintf(w, "git worktree add %s\n", filepath.Base(wtPath))

	start := time.Now()

	var buf bytes.Buffer

	// No --progress flag: `git worktree add` has no such option, and it reports
	// progress only to a TTY anyway — which this never is, since output is piped
	// into the TUI. The elapsed-time line below is what makes the wait legible.
	cmd := exec.Command("git", "worktree", "add", wtPath, "-b", branch)
	cmd.Dir = repoDir
	cmd.Stdout = io.MultiWriter(w, &buf)
	cmd.Stderr = cmd.Stdout

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git worktree add: %s", strings.TrimSpace(buf.String()))
	}

	fmt.Fprintf(w, "checkout done in %s\n", time.Since(start).Round(time.Second))

	// .env is gitignored, so a fresh worktree has none and generate would build one
	// from the Laravel template — dropping every project-specific key (API creds,
	// SUPER_ADMIN_EMAIL, MEILI_SEARCH_KEY...) and minting a new APP_KEY, which makes
	// any encrypted column in a cloned database unreadable. Copy the main checkout's
	// instead; WriteEnv then patches only the frank-managed keys on top.
	copyIfExists(repoDir, wtPath, ".env")
	copyIfExists(repoDir, wtPath, "auth.json")

	// vendor/ and node_modules/ are gitignored, so a fresh worktree has neither and
	// laravel.migrate runs a full `composer install` (and the vite sidecar an `npm
	// install`) on first up — minutes of wall-clock. Copying them is instant on any
	// CoW filesystem and still far cheaper than reinstalling elsewhere.
	for _, dir := range []string{"vendor", "node_modules"} {
		if err := copyTree(filepath.Join(repoDir, dir), filepath.Join(wtPath, dir), w); err != nil {
			fmt.Fprintf(w, "warning: could not copy %s (%v) — it will be reinstalled on first up\n", dir, err)
		}
	}

	if seedDB {
		if err := markSeedDB(wtPath, repoDir); err != nil {
			return fmt.Errorf("mark database clone: %w", err)
		}
	}

	return nil
}

// copyTree copies src to dst, preferring a copy-on-write clone. --reflink=auto is
// GNU-only and silently falls back to a real copy on filesystems without CoW; the
// second attempt covers platforms whose cp does not know the flag at all.
func copyTree(src, dst string, w io.Writer) error {
	if _, err := os.Stat(src); err != nil {
		return nil // nothing to copy
	}

	fmt.Fprintf(w, "copying %s\n", filepath.Base(src))

	if err := exec.Command("cp", "-a", "--reflink=auto", src, dst).Run(); err == nil {
		return nil
	}

	out, err := exec.Command("cp", "-a", src, dst).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}

	return nil
}

func copyIfExists(srcDir, dstDir, name string) {
	src := filepath.Join(srcDir, name)

	data, err := os.ReadFile(src)
	if err != nil {
		return
	}

	info, _ := os.Stat(src)
	perm := os.FileMode(0644)

	if info != nil {
		perm = info.Mode().Perm()
	}

	_ = os.WriteFile(filepath.Join(dstDir, name), data, perm)
}

func regenerate(path string, w io.Writer) error {
	return runFrank(w, "generate", "--dir", path)
}

// progressMsg carries one line of a running action's output to the TUI.
type progressMsg struct{ line string }

// lineWriter splits streamed command output into lines and forwards each one to
// the bubbletea program. Compose and output.Region redraw in place with \r, so
// \r counts as a line break too. A nil send makes it a sink (tests).
type lineWriter struct {
	send   func(tea.Msg)
	prefix string
	buf    []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)

	for {
		i := bytes.IndexAny(w.buf, "\r\n")
		if i < 0 {
			break
		}

		line := strings.TrimSpace(stripANSI(string(w.buf[:i])))
		w.buf = w.buf[i+1:]

		if line != "" && w.send != nil {
			w.send(progressMsg{line: w.prefix + line})
		}
	}

	return len(p), nil
}

var ansiSeq = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

func stripANSI(s string) string { return ansiSeq.ReplaceAllString(s, "") }
