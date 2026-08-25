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

func RemoveWorktree(path, branch string, w io.Writer) error {
	_ = docker.New(path).RunStream(w, "down")

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

	return runFrank(w, "up", "-d", "--dir", path)
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

func CreateWorktree(repoDir, wtPath, branch string) error {
	cmd := exec.Command("git", "worktree", "add", wtPath, "-b", branch)
	cmd.Dir = repoDir

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add: %s", out)
	}

	copyIfExists(repoDir, wtPath, "auth.json")

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
	send func(tea.Msg)
	buf  []byte
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
			w.send(progressMsg{line: line})
		}
	}

	return len(p), nil
}

var ansiSeq = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

func stripANSI(s string) string { return ansiSeq.ReplaceAllString(s, "") }
