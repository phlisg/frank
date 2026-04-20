package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/tool"
	"github.com/spf13/cobra"
)

func init() {
	toolCmd.AddCommand(toolAddCmd)
}

var toolAddCmd = &cobra.Command{
	Use:   "add <tool>",
	Short: "Add a dev tool to frank.yaml and install its config files",
	Args:  cobra.ExactArgs(1),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return tool.AllNames(), cobra.ShellCompDirectiveNoFileComp
	},
	SilenceUsage: true,
	RunE:         runToolAdd,
}

func runToolAdd(cmd *cobra.Command, args []string) error {
	name := args[0]

	if !tool.Valid(name) {
		return fmt.Errorf("unknown tool %q — valid options: %v", name, tool.AllNames())
	}

	dir := resolveDir()

	cfg, err := config.Load(dir)
	if err != nil {
		return err
	}

	if slices.Contains(cfg.Tools, name) {
		return fmt.Errorf("tool %q is already configured", name)
	}

	cfg.Tools = append(cfg.Tools, name)

	if err := saveConfig(cfg, dir); err != nil {
		return err
	}

	fmt.Printf("  added    %s\n", name)

	if _, err := tool.Install([]string{name}, dir); err != nil {
		return err
	}

	hint := tool.LefthookHint(name)
	if hint != "" {
		lefthookPath := filepath.Join(dir, "lefthook.yml")
		if _, err := os.Stat(lefthookPath); err == nil {
			if slices.Contains(cfg.Tools, "lefthook") {
				fmt.Printf("  hint     add this to lefthook.yml pre-commit commands:\n%s\n", hint)
			}
		}
	}

	return nil
}
