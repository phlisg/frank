package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/docker"
	"github.com/spf13/cobra"
)

var forceReset bool

func init() {
	resetCmd.Flags().BoolVar(&forceReset, "force", false, "skip confirmation prompt")
	rootCmd.AddCommand(resetCmd)
}

// preservedFiles are kept during reset; everything else is deleted.
var preservedFiles = map[string]bool{
	".git":          true,
	"frank.yaml":    true,
	".dockerignore": true,
	".gitignore":    true,
	"README.md":     true,
}

var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Remove all project files except frank.yaml and .git (destructive)",
	Long: `Stops containers, removes volumes, then deletes all project files except:
  .git/  frank.yaml  .dockerignore  .gitignore  README.md

Use --force to skip the confirmation prompt.`,
	SilenceUsage:      true,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE:              runReset,
}

func runReset(cmd *cobra.Command, args []string) error {
	dir := resolveDir()

	// Load config before deletion (frank.yaml is preserved so it survives reset).
	cfg, cfgErr := config.Load(dir)

	if !forceReset {
		fmt.Printf("This will delete all files in %s except preserved files. Continue? [y/N] ", dir)
		var answer string
		fmt.Scanln(&answer)
		if answer != "y" && answer != "Y" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	if err := docker.New(dir).Clean(); err != nil {
		fmt.Printf("warning: docker clean failed: %v\n", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read directory: %w", err)
	}

	for _, entry := range entries {
		if preservedFiles[entry.Name()] {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if err := os.RemoveAll(path); err != nil {
			fmt.Printf("warning: could not remove %s: %v\n", entry.Name(), err)
		} else {
			fmt.Printf("  removed  %s\n", entry.Name())
		}
	}

	// Restore .gitignore from git if it was modified.
	restoreGitignore(dir)

	// Regenerate .frank/ Docker files from the preserved frank.yaml.
	if cfgErr != nil {
		fmt.Printf("warning: could not load frank.yaml, skipping generate: %v\n", cfgErr)
	} else {
		fmt.Println("Regenerating Docker files...")
		if err := generate(cfg, dir); err != nil {
			fmt.Printf("warning: generate failed: %v\n", err)
		}
	}

	fmt.Println("Reset complete.")
	return nil
}

// restoreGitignore restores .gitignore to the git-tracked version if it has
// been modified. Fails silently — not all projects are git repos.
func restoreGitignore(dir string) {
	cmd := exec.Command("git", "checkout", "--", ".gitignore")
	cmd.Dir = dir
	if err := cmd.Run(); err == nil {
		fmt.Println("  restored .gitignore from git")
	}
}
