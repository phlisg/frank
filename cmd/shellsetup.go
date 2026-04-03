package cmd

import (
	"fmt"

	"github.com/phlisg/frank/internal/shell"
	"github.com/spf13/cobra"
)

var shellName string

func init() {
	shellSetupCmd.Flags().StringVar(&shellName, "shell", "", "shell type: zsh or bash (auto-detected if not set)")
	rootCmd.AddCommand(shellSetupCmd)
}

var shellSetupCmd = &cobra.Command{
	Use:   "shell-setup",
	Short: "Output shell hook for auto-activating on cd (eval this)",
	Long: `Outputs an eval-able chpwd hook (zsh) or cd wrapper (bash) that
automatically runs 'frank activate' when you enter a directory containing frank.yaml.

Add this to your shell profile:
  eval "$(frank shell-setup)"`,
	SilenceUsage:      true,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Print(shell.ShellSetup(shellName))
		return nil
	},
}
