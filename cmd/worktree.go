package cmd

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/phlisg/frank/internal/docker"
	"github.com/phlisg/frank/internal/output"
	"github.com/spf13/cobra"
)

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

		// Remove the git worktree.
		gitCmd := exec.Command("git", "worktree", "remove", absPath)
		if out, err := gitCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git worktree remove failed: %s", out)
		}

		output.Group("Worktree removed", absPath)
		return nil
	},
}
