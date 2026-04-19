package cmd

import (
	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/docker"
	"github.com/spf13/cobra"
)

var psWorkers bool

func init() {
	psCmd.Flags().BoolVar(&psWorkers, "workers", false, "Show only worker containers (declared and ad-hoc)")
	rootCmd.AddCommand(psCmd)
}

var psCmd = &cobra.Command{
	Use:               "ps",
	Short:             "Show running containers",
	SilenceUsage:      true,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := resolveDir()
		if psWorkers {
			return docker.New(dir).PSWorkers(config.ProjectName(dir))
		}
		return docker.New(dir).PS()
	},
}
