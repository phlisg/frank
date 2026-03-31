package cmd

import (
	"github.com/phlisg/frank/internal/docker"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(psCmd)
}

var psCmd = &cobra.Command{
	Use:          "ps",
	Short:        "Show running containers",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return docker.New(resolveDir()).PS()
	},
}
