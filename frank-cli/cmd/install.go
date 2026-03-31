package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/phlisg/frank-cli/internal/config"
	"github.com/phlisg/frank-cli/internal/docker"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(installCmd)
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install a new Laravel project inside the container",
	Long: `Runs the Laravel installer inside the laravel.test container — no local PHP needed.

Steps:
  1. Back up README.md and .gitignore
  2. composer create-project laravel/laravel . <version>
  3. Restore README.md and .gitignore
  4. Overwrite .env and .env.example with Frank-generated versions
  5. Patch vite.config.js for Docker HMR (server.host = '0.0.0.0')
  6. Copy .psysh.php if present`,
	SilenceUsage: true,
	RunE:         runInstall,
}

func runInstall(cmd *cobra.Command, args []string) error {
	dir := resolveDir()

	cfg, err := config.Load(dir)
	if err != nil {
		return err
	}

	d := docker.New(dir)

	// 1. Back up Frank's README.md and .gitignore before composer overwrites them.
	restore, err := backupFiles(dir, "README.md", ".gitignore")
	if err != nil {
		return fmt.Errorf("backup files: %w", err)
	}

	// 2. composer create-project
	laravelVersion := cfg.Laravel.Version
	if laravelVersion == "latest" {
		laravelVersion = ""
	}

	createArgs := []string{"create-project", "laravel/laravel", "."}
	if laravelVersion != "" && laravelVersion != "lts" {
		createArgs = append(createArgs, laravelVersion)
	}
	createArgs = append(createArgs, "--no-interaction")

	fmt.Println("Installing Laravel...")
	if err := d.Exec("laravel.test", append([]string{"composer"}, createArgs...)...); err != nil {
		return fmt.Errorf("composer create-project: %w", err)
	}

	// 3. Restore Frank's README.md and .gitignore.
	if err := restore(); err != nil {
		return fmt.Errorf("restore files: %w", err)
	}

	// 4. Regenerate Docker files so .env/.env.example reflect Frank's service config.
	fmt.Println("Regenerating Docker files...")
	if err := generate(cfg, dir); err != nil {
		return err
	}

	// 5. Patch vite.config.js for Docker HMR.
	if err := patchViteConfig(dir); err != nil {
		fmt.Printf("warning: could not patch vite.config.js: %v\n", err)
	}

	// 6. Copy .psysh.php from project root if present.
	if err := copyPsysh(dir); err != nil {
		fmt.Printf("warning: could not copy .psysh.php: %v\n", err)
	}

	fmt.Println("Laravel installed successfully.")
	fmt.Println("Run 'frank up' to start your project.")
	return nil
}

// patchViteConfig adds server.host = '0.0.0.0' to vite.config.js for Docker HMR.
func patchViteConfig(dir string) error {
	path := filepath.Join(dir, "vite.config.js")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // nothing to patch
		}
		return err
	}

	content := string(data)

	// Already patched.
	if strings.Contains(content, "server.host") || strings.Contains(content, "'0.0.0.0'") {
		return nil
	}

	// Insert server config into the defineConfig block.
	patched := strings.Replace(
		content,
		"defineConfig({",
		"defineConfig({\n    server: { host: '0.0.0.0' },",
		1,
	)

	if patched == content {
		// Pattern not found — leave the file alone rather than corrupt it.
		return nil
	}

	if err := os.WriteFile(path, []byte(patched), 0644); err != nil {
		return err
	}
	fmt.Println("  patched  vite.config.js (server.host = '0.0.0.0')")
	return nil
}

// copyPsysh copies .psysh.php from dir into the project if it exists.
// (Frank ships a default .psysh.php alongside frank.yaml for nicer tinker sessions.)
func copyPsysh(dir string) error {
	src := filepath.Join(dir, ".psysh.php")
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil // nothing to copy
	}
	fmt.Println("  .psysh.php already in place")
	return nil
}

// backupFiles reads the named files from dir (if they exist) and returns a
// restore func that writes them back. Files that did not exist before are
// deleted on restore so composer's generated versions don't linger.
func backupFiles(dir string, names ...string) (func() error, error) {
	type snapshot struct {
		path    string
		content []byte // nil means the file did not exist
	}

	snaps := make([]snapshot, 0, len(names))
	for _, name := range names {
		p := filepath.Join(dir, name)
		data, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				snaps = append(snaps, snapshot{path: p, content: nil})
				continue
			}
			return nil, err
		}
		snaps = append(snaps, snapshot{path: p, content: data})
	}

	restore := func() error {
		for _, s := range snaps {
			if s.content == nil {
				// File did not exist before — remove what composer created.
				if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
					return err
				}
				continue
			}
			if err := os.WriteFile(s.path, s.content, 0644); err != nil {
				return err
			}
			fmt.Printf("  restored %s\n", filepath.Base(s.path))
		}
		return nil
	}

	return restore, nil
}
