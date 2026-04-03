package cmd

import (
	"fmt"

	"github.com/phlisg/frank/internal/shell"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(deactivateCmd)
}

var deactivateCmd = &cobra.Command{
	Use:               "deactivate",
	Short:             "Output unalias commands to remove frank aliases (eval this)",
	SilenceUsage:      true,
	ValidArgsFunction: cobra.NoFileCompletions,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(shell.Deactivate())
	},
}
