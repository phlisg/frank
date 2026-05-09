package cmd

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/phlisg/frank/internal/docker"
	"github.com/phlisg/frank/internal/output"
	"github.com/spf13/cobra"
)

func branchForPath(porcelain, path string) string {
	var current string
	for _, line := range strings.Split(porcelain, "\n") {
		if strings.HasPrefix(line, "worktree ") {
			current = strings.TrimPrefix(line, "worktree ")
		}
		if current == path && strings.HasPrefix(line, "branch refs/heads/") {
			return strings.TrimPrefix(line, "branch refs/heads/")
		}
	}
	return ""
}

func init() {
	rootCmd.AddCommand(worktreeCmd)
	worktreeCmd.AddCommand(worktreeRemoveCmd)
}

var worktreeCmd = &cobra.Command{
	Use:   "worktree",
	Short: "Manage git worktrees",
	ValidArgsFunction: cobra.NoFileCompletions,
}

var worktreeRemoveCmd = &cobra.Command{
	Use:               "remove <path>",
	Short:             "Tear down containers and remove a git worktree",
	Args:              cobra.ExactArgs(1),
	SilenceUsage:      true,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(cmd *cobra.Command, args []string) error {
		absPath, err := filepath.Abs(args[0])
		if err != nil {
			return err
		}

		// Tear down any running compose project in the worktree.
		// Ignore errors — containers may not be running.
		client := docker.New(absPath)
		_ = client.Down()

		// Resolve branch name before removing.
		branchCmd := exec.Command("git", "worktree", "list", "--porcelain")
		branchOut, _ := branchCmd.Output()
		branch := branchForPath(string(branchOut), absPath)

		// Remove the git worktree.
		gitCmd := exec.Command("git", "worktree", "remove", "--force", absPath)
		if out, err := gitCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git worktree remove failed: %s", out)
		}

		if branch != "" {
			_ = exec.Command("git", "branch", "-D", branch).Run()
		}

		output.Group("Worktree removed", absPath)
		return nil
	},
}
