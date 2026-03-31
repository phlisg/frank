package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "frank",
	Short: "Frank — Laravel Development Environment",
	// Smart contextual help is implemented in cmd/root_run.go once
	// internal/config and internal/docker packages are available.
	// For now, default Cobra help is used.
}

// Dir is the global --dir flag value (target project directory).
var Dir string

func init() {
	rootCmd.PersistentFlags().StringVar(&Dir, "dir", "", "target directory (defaults to current working directory)")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
