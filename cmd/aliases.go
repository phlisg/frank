package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/shell"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(aliasesCmd)
}

var aliasesCmd = &cobra.Command{
	Use:               "aliases",
	Short:             "List all shell aliases for this project",
	SilenceUsage:      true,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := resolveDir()
		cfg, err := config.Load(dir)
		if err != nil {
			return fmt.Errorf("no %s found in %s", config.ConfigFileName, filepath.Base(dir))
		}

		entries := shell.ListAliases(cfg)

		hasBuiltin := false
		hasCustom := false
		for _, e := range entries {
			if e.Custom {
				hasCustom = true
			} else {
				hasBuiltin = true
			}
		}

		maxName := 0
		for _, e := range entries {
			if len(e.Name) > maxName {
				maxName = len(e.Name)
			}
		}

		if hasBuiltin {
			fmt.Println("Built-in:")
			for _, e := range entries {
				if e.Custom {
					continue
				}
				note := ""
				if e.Note != "" {
					note = fmt.Sprintf("  (%s)", e.Note)
				}
				fmt.Printf("  %-*s  → %s%s\n", maxName, e.Name, shortCmd(e.Cmd), note)
			}
		}

		if hasCustom {
			if hasBuiltin {
				fmt.Println()
			}
			fmt.Println("Custom (frank.yaml):")
			for _, e := range entries {
				if !e.Custom {
					continue
				}
				note := ""
				if e.Note != "" {
					note = fmt.Sprintf("  (%s)", e.Note)
				}
				fmt.Printf("  %-*s  → %s%s\n", maxName, e.Name, shortCmd(e.Cmd), note)
			}
		}

		return nil
	},
}

// shortCmd trims the verbose docker compose prefix for readability.
func shortCmd(cmd string) string {
	const dcPrefix = "docker compose --project-directory . -f .frank/compose.yaml "
	const execPrefix = dcPrefix + "exec --user sail laravel.test "
	switch {
	case len(cmd) > len(execPrefix) && cmd[:len(execPrefix)] == execPrefix:
		return cmd[len(execPrefix):]
	case len(cmd) > len(dcPrefix) && cmd[:len(dcPrefix)] == dcPrefix:
		return "dc " + cmd[len(dcPrefix):]
	default:
		return cmd
	}
}
