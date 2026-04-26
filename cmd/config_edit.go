package cmd

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/phlisg/frank/internal/config"
	"github.com/spf13/cobra"
)

func init() {
	configCmd.AddCommand(configEditCmd)
}

var configEditCmd = &cobra.Command{
	Use:               "edit",
	Short:             "Open frank.yaml in your editor",
	SilenceUsage:      true,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := resolveDir()
		path := filepath.Join(dir, config.ConfigFileName)
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = os.Getenv("VISUAL")
		}
		if editor == "" {
			editor = "vi"
		}
		c := exec.Command(editor, path)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	},
}
