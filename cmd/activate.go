package cmd

import (
	"fmt"

	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/shell"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(activateCmd)
}

var activateCmd = &cobra.Command{
	Use:               "activate",
	Short:             "Output shell aliases for the current project (eval this)",
	SilenceUsage:      true,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := resolveDir()
		cfg, err := config.Load(dir)
		if err != nil {
			return fmt.Errorf("no frank.yaml found — run frank init first")
		}
		fmt.Print(shell.Activate(cfg))
		return nil
	},
}
