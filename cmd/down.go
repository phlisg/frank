package cmd

import (
	"fmt"
	"os"

	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/docker"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(downCmd)
}

var downCmd = &cobra.Command{
	Use:               "down",
	Short:             "Stop containers",
	SilenceUsage:      true,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := resolveDir()
		client := docker.New(dir)

		// Stop ad-hoc workers first so `docker compose down` doesn't leave
		// them behind as orphans. Failures here are warned, not fatal —
		// the user can still tear down the main stack.
		project := config.ProjectName(dir)
		if names, err := client.AdhocWorkerNames(project); err == nil && len(names) > 0 {
			fmt.Printf("Removing ad-hoc workers: %v\n", names)
			if err := client.StopContainers(names); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not remove ad-hoc workers: %v\n", err)
			}
		}

		return client.Down()
	},
}
