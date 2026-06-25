package cmd

import (
	"github.com/phlisg/frank/internal/docker"
	"github.com/spf13/cobra"
)

func init() {
	devCmd.AddCommand(devRestartCmd, devStopCmd, devStartCmd)
	rootCmd.AddCommand(devCmd)
}

// devCmd with no subcommand tails the laravel.vite logs. Ctrl-C detaches; the
// container keeps running (it lives in compose.yaml, started by `frank up`).
var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Attach to the frontend dev server (Vite)",
	Long: `Tail the laravel.vite dev-server logs. Ctrl-C detaches without stopping it.

The dev server runs as a compose sidecar (laravel.vite), started by frank up and
stopped by frank down. Disable it per-project with dev.enabled: false in frank.yaml.`,
	SilenceUsage:      true,
	Args:              cobra.NoArgs,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(cmd *cobra.Command, args []string) error {
		return docker.New(resolveDir()).Run("logs", "-f", "--no-log-prefix", "laravel.vite")
	},
}

var devRestartCmd = &cobra.Command{
	Use:               "restart",
	Short:             "Restart the dev server",
	SilenceUsage:      true,
	Args:              cobra.NoArgs,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(cmd *cobra.Command, args []string) error {
		return docker.New(resolveDir()).Run("restart", "laravel.vite")
	},
}

var devStopCmd = &cobra.Command{
	Use:               "stop",
	Short:             "Stop the dev server",
	SilenceUsage:      true,
	Args:              cobra.NoArgs,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(cmd *cobra.Command, args []string) error {
		return docker.New(resolveDir()).Run("stop", "laravel.vite")
	},
}

var devStartCmd = &cobra.Command{
	Use:               "start",
	Short:             "Start the dev server",
	SilenceUsage:      true,
	Args:              cobra.NoArgs,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(cmd *cobra.Command, args []string) error {
		return docker.New(resolveDir()).Run("start", "laravel.vite")
	},
}
