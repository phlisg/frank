package cmd

import (
	"github.com/phlisg/frank-cli/internal/docker"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(downCmd)
}

var downCmd = &cobra.Command{
	Use:          "down",
	Short:        "Stop containers",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return docker.New(resolveDir()).Down()
	},
}
