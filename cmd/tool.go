package cmd

import "github.com/spf13/cobra"

func init() {
	rootCmd.AddCommand(toolCmd)
}

var toolCmd = &cobra.Command{
	Use:   "tool",
	Short: "Manage dev tools (pint, larastan, rector, lefthook)",
}
