package cmd

import (
	"fmt"

	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/shell"
	"github.com/spf13/cobra"
)

var shellName string

var configShellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Shell integration (aliases, hooks, completion)",
}

var configShellActivateCmd = &cobra.Command{
	Use:               "activate",
	Short:             "Output shell aliases for the current project (eval this)",
	SilenceUsage:      true,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := resolveDir()
		cfg, err := config.Load(dir)
		if err != nil {
			return fmt.Errorf("no frank.yaml found — run frank new or frank setup first")
		}
		fmt.Print(shell.Activate(cfg))
		return nil
	},
}

var configShellDeactivateCmd = &cobra.Command{
	Use:               "deactivate",
	Short:             "Output unalias commands to remove frank aliases (eval this)",
	SilenceUsage:      true,
	ValidArgsFunction: cobra.NoFileCompletions,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(shell.Deactivate())
	},
}

var configShellSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Output shell hook for auto-activating on cd (eval this)",
	Long: `Outputs an eval-able chpwd hook (zsh) or cd wrapper (bash) that
automatically runs 'frank activate' when you enter a directory containing frank.yaml.

Add this to your shell profile:
  eval "$(frank config shell setup)"`,
	SilenceUsage:      true,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Print(shell.ShellSetup(shellName))
		return nil
	},
}

var configShellCompletionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion script",
	Long: `Generate shell completion script for frank.

Add this to your shell profile (or let frank config shell setup handle it automatically):
  # zsh
  eval "$(frank config shell completion zsh)"
  # bash
  eval "$(frank config shell completion bash)"`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return []string{"bash", "zsh", "fish", "powershell"}, cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
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

func init() {
	configShellSetupCmd.Flags().StringVar(&shellName, "shell", "", "shell type: zsh or bash (auto-detected if not set)")

	configCmd.AddCommand(configShellCmd)
	configShellCmd.AddCommand(configShellActivateCmd)
	configShellCmd.AddCommand(configShellDeactivateCmd)
	configShellCmd.AddCommand(configShellSetupCmd)
	configShellCmd.AddCommand(configShellCompletionCmd)
}
