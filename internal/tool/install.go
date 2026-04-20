package tool

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"

	"github.com/phlisg/frank/internal/output"
)

type InstallResult struct {
	Created []string
	Skipped []string
}

func Install(tools []string, dir string) (*InstallResult, error) {
	if err := ValidateNames(tools); err != nil {
		return nil, err
	}

	res := &InstallResult{}

	for _, name := range tools {
		t, _ := Lookup(name)

		for dest, src := range t.ConfigFiles {
			destPath := filepath.Join(dir, dest)
			if _, err := os.Stat(destPath); err == nil {
				output.Detail(fmt.Sprintf("skipped %s (already exists)", dest))
				res.Skipped = append(res.Skipped, dest)
				continue
			}

			data, err := configFS.ReadFile("files/" + src)
			if err != nil {
				return nil, fmt.Errorf("read embedded %s: %w", src, err)
			}

			if err := os.WriteFile(destPath, data, 0644); err != nil {
				return nil, fmt.Errorf("write %s: %w", dest, err)
			}
			output.Detail(fmt.Sprintf("created %s", dest))
			res.Created = append(res.Created, dest)
		}
	}

	hasLefthook := slices.Contains(tools, "lefthook")

	if hasLefthook {
		lefthookPath := filepath.Join(dir, "lefthook.yml")
		if _, err := os.Stat(lefthookPath); err == nil {
			output.Detail("skipped lefthook.yml (already exists)")
			res.Skipped = append(res.Skipped, "lefthook.yml")
		} else {
			content := AssembleLefthook(tools)
			if err := os.WriteFile(lefthookPath, []byte(content), 0644); err != nil {
				return nil, fmt.Errorf("write lefthook.yml: %w", err)
			}
			output.Detail("created lefthook.yml")
			res.Created = append(res.Created, "lefthook.yml")
		}
	}

	phpTools := PHPTools(tools)
	if len(phpTools) > 0 {
		if err := PatchComposerScripts(dir, phpTools); err != nil {
			return nil, err
		}
	}

	if hasLefthook {
		runLefthookInstall(dir)
	}

	return res, nil
}

func runLefthookInstall(dir string) {
	gitDir := filepath.Join(dir, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		initCmd := exec.Command("git", "init")
		initCmd.Dir = dir
		if err := initCmd.Run(); err != nil {
			output.Detail("hint: run `lefthook install` after git init")
			return
		}
	}
	path, err := exec.LookPath("lefthook")
	if err != nil {
		output.Detail("hint: install lefthook to enable git hooks: https://github.com/evilmartians/lefthook")
		return
	}
	cmd := exec.Command(path, "install")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		output.Warning(fmt.Sprintf("lefthook install failed: %v", err))
	}
}

func LefthookHint(toolName string) string {
	t, ok := Lookup(toolName)
	if !ok || t.Category != "php" {
		return ""
	}
	return lefthookEntry(t)
}
