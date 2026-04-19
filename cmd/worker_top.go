package cmd

import (
	"fmt"

	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/docker"
	"github.com/phlisg/frank/internal/workertop"
	"github.com/spf13/cobra"
)

var (
	workerTopLive         bool
	workerTopMinPaneWidth int
)

func init() {
	workerTopCmd.Flags().BoolVar(&workerTopLive, "live", false,
		"Reconcile ad-hoc workers every 2s (add/remove panes as they spawn/exit)")
	workerTopCmd.Flags().IntVar(&workerTopMinPaneWidth, "min-pane-width", 30,
		"Minimum column width before pane titles truncate")

	workerCmd.AddCommand(workerTopCmd)
}

var workerTopCmd = &cobra.Command{
	Use:               "top",
	Short:             "Live multi-pane view of schedule + queue + ad-hoc workers",
	SilenceUsage:      true,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE:              runWorkerTop,
}

func runWorkerTop(cmd *cobra.Command, _ []string) error {
	dir := resolveDir()
	cfg, err := config.Load(dir)
	if err != nil {
		return fmt.Errorf("load frank.yaml: %w", err)
	}
	projectName := config.ProjectName(dir)
	client := docker.New(dir)

	opts := workertop.Opts{
		Live:         workerTopLive,
		MinPaneWidth: workerTopMinPaneWidth,
	}

	return workertop.Run(cmd.Context(), cfg, projectName, client, opts)
}
