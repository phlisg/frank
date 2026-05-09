package worktreelist

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

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

func RemoveWorktree(path, branch string) error {
	_ = docker.New(path).Down()

	out, err := exec.Command("git", "worktree", "remove", "--force", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree remove: %s", out)
	}

	if branch != "" && branch != "(detached)" {
		_ = exec.Command("git", "branch", "-D", branch).Run()
	}
	return nil
}

func upContainers(path string) error {
	frank, err := os.Executable()
	if err != nil {
		frank = "frank"
	}
	out, err := exec.Command(frank, "up", "-d", "--dir", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("frank up: %s", out)
	}
	return nil
}

func downContainers(path string) error {
	frank, err := os.Executable()
	if err != nil {
		frank = "frank"
	}
	out, err := exec.Command(frank, "down", "--dir", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("frank down: %s", out)
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
	return nil
}

func regenerate(path string) error {
	frank, err := os.Executable()
	if err != nil {
		frank = "frank"
	}
	out, err := exec.Command(frank, "generate", "--dir", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("frank generate: %s", out)
	}
	return nil
}
