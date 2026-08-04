package cmd

import (
	"fmt"
	"os"

	"github.com/phlisg/frank/internal/activeproject"
	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/docker"
	"github.com/phlisg/frank/internal/output"
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
		// Session log stays here rather than in doDown: `frank up` calls
		// doDown for a *different* project's dir, and must keep writing to
		// its own project's debug.log.
		defer openSessionAppend(dir)()
		return doDown(dir)
	},
}

// doDown tears down the project in dir. Shared with `frank up`, which calls it
// to stop the previously active project before starting its own.
func doDown(dir string) error {
	client := docker.New(dir)

	// Stop the detached watcher first so it doesn't fire one last
	// queue:restart against the containers we're about to remove.
	// Non-fatal: a missing pidfile just means no watcher.
	if stopped, _, err := runWatchStop(dir); err != nil {
		output.Warning(fmt.Sprintf("could not stop watcher: %v", err))
	} else if stopped {
		output.Group("Stopped file watcher", "")
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

	// `docker compose down` runs without --rmi, so the shared base
	// (frank/runtime:*) is NOT removed — other Frank projects depend on it.
	// Do not add --rmi here.
	region := output.Region("Stopping containers")
	err := client.RunStream(region, "down")
	region.Stop(err)
	if err != nil {
		return err
	}

	// Worktrees publish ephemeral ports and can co-exist, so they never own
	// the active-project pointer — leave whatever it names alone. Clear is a
	// no-op when the pointer names some other project.
	if !config.IsWorktree(dir) {
		if err := activeproject.Clear(dir); err != nil {
			// Bookkeeping only: the containers are already down, so this
			// must not change down's exit status.
			output.Warning(fmt.Sprintf("could not clear active project: %v", err))
		}
	}

	return nil
}
