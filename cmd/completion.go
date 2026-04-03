package cmd

import (
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(completionCmd)
}

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion script",
	Long: `Generate shell completion script for frank.

Add this to your shell profile (or let frank shell-setup handle it automatically):
  # zsh
  eval "$(frank completion zsh)"
  # bash
  eval "$(frank completion bash)"`,
	DisableFlagParsing: false,
	ValidArgs:          []string{"bash", "zsh", "fish", "powershell"},
	Args:               cobra.ExactArgs(1),
	SilenceUsage:       true,
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletion(cmd.OutOrStdout())
		case "zsh":
			return rootCmd.GenZshCompletion(cmd.OutOrStdout())
		case "fish":
			return rootCmd.GenFishCompletion(cmd.OutOrStdout(), true)
		case "powershell":
			return rootCmd.GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
		default:
			return cmd.Help()
		}
	},
}
