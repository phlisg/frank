package cmd

import (
	"github.com/phlisg/frank/internal/docker"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(cleanCmd)
}

var cleanCmd = &cobra.Command{
	Use:          "clean",
	Short:        "Stop containers and remove volumes",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return docker.New(resolveDir()).Clean()
	},
}
