package cmd

import (
	"fmt"

	"github.com/phlisg/frank/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var configShowCmd = &cobra.Command{
	Use:               "show",
	Short:             "Show resolved configuration",
	SilenceUsage:      true,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := resolveDir()
		cfg, err := config.Load(dir)
		if err != nil {
			return err
		}
		out, err := yaml.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("marshal config: %w", err)
		}
		fmt.Print(string(out))
		return nil
	},
}

func init() {
	configCmd.AddCommand(configShowCmd)
}
