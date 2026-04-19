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

		// Stop the detached watcher first so it doesn't fire one last
		// queue:restart against the containers we're about to remove.
		// Non-fatal: a missing pidfile just means no watcher.
		if err := runWatchStop(dir); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not stop watcher: %v\n", err)
		}

		// Stop ad-hoc workers so `docker compose down` doesn't leave
		// them behind as orphans. Failures here are warned, not fatal.
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
