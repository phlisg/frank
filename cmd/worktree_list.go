package cmd

import (
	"github.com/phlisg/frank/internal/worktreelist"
	"github.com/spf13/cobra"
)

func init() {
	worktreeCmd.AddCommand(worktreeListCmd)
}

var worktreeListCmd = &cobra.Command{
	Use:               "list",
	Short:             "Interactive list of linked git worktrees",
	SilenceUsage:      true,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := resolveDir()

		items, err := worktreelist.Discover(dir)
		if err != nil {
			return err
		}
		return worktreelist.Run(dir, items)
	},
}
