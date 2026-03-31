package cmd

import (
	"io/fs"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "frank",
	Short: "Frank — Laravel Development Environment",
	// Smart contextual help is implemented once internal/config and
	// internal/docker packages are available.
}

// Dir is the global --dir flag value (target project directory).
var Dir string

// TemplateFS holds the embedded templates FS passed from main.
var TemplateFS fs.FS

func init() {
	rootCmd.PersistentFlags().StringVar(&Dir, "dir", "", "target directory (defaults to current working directory)")
}

func Execute(fsys fs.FS) {
	TemplateFS = fsys
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
