package tool

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
)

type InstallResult struct {
	Created []string
	Skipped []string
	Patched bool
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
				fmt.Printf("  skipped    %s (already exists)\n", dest)
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
			fmt.Printf("  created    %s\n", dest)
			res.Created = append(res.Created, dest)
		}
	}

	hasLefthook := slices.Contains(tools, "lefthook")

	if hasLefthook {
		lefthookPath := filepath.Join(dir, "lefthook.yml")
		if _, err := os.Stat(lefthookPath); err == nil {
			fmt.Println("  skipped    lefthook.yml (already exists)")
			res.Skipped = append(res.Skipped, "lefthook.yml")
		} else {
			content := AssembleLefthook(tools)
			if err := os.WriteFile(lefthookPath, []byte(content), 0644); err != nil {
				return nil, fmt.Errorf("write lefthook.yml: %w", err)
			}
			fmt.Println("  created    lefthook.yml")
			res.Created = append(res.Created, "lefthook.yml")
		}
	}

	phpTools := PHPTools(tools)
	if len(phpTools) > 0 {
		patched, err := PatchComposer(dir, phpTools)
		if err != nil {
			return nil, err
		}
		res.Patched = patched
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
			fmt.Println("  hint       run `lefthook install` after git init")
			return
		}
	}
	path, err := exec.LookPath("lefthook")
	if err != nil {
		fmt.Println("  hint       install lefthook to enable git hooks: https://github.com/evilmartians/lefthook")
		return
	}
	cmd := exec.Command(path, "install")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "  warning    lefthook install failed: %v\n", err)
	}
}

func LefthookHint(toolName string) string {
	t, ok := Lookup(toolName)
	if !ok || t.Category != "php" {
		return ""
	}
	return lefthookEntry(t)
}
