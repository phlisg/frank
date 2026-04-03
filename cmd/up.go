package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/docker"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(upCmd)
}

var upCmd = &cobra.Command{
	Use:   "up [docker compose flags]",
	Short: "Start containers (passes flags through to docker compose up)",
	Long: `Start containers by passing all flags directly to docker compose up.

Examples:
  frank up              # foreground
  frank up -d           # detached
  frank up -d --build   # detached, force rebuild

Frank-specific flag:
  --quick   Skip post-start tasks (composer install + artisan migrate)`,
	DisableFlagParsing: true,
	SilenceUsage:       true,
	ValidArgsFunction:  cobra.NoFileCompletions,
	RunE:               runUp,
}

func runUp(cmd *cobra.Command, args []string) error {
	dir := resolveDir()
	client := docker.New(dir)

	quick := false
	var composeArgs []string
	for _, a := range args {
		if a == "--quick" {
			quick = true
		} else if a == "--help" || a == "-h" {
			return cmd.Help()
		} else {
			composeArgs = append(composeArgs, a)
		}
	}

	// Pre-flight: ensure .frank/ has been generated
	if _, err := os.Stat(filepath.Join(dir, ".frank", "compose.yaml")); os.IsNotExist(err) {
		return fmt.Errorf("no Docker config found — run frank generate first")
	}

	// Pre-flight: detect runtime/PHP version change since last generate
	if stateData, err := os.ReadFile(filepath.Join(dir, ".frank", ".state")); err == nil {
		var state struct {
			PHPVersion string `json:"phpVersion"`
			Runtime    string `json:"runtime"`
		}
		if err := json.Unmarshal(stateData, &state); err == nil {
			cfg, err := config.Load(dir)
			if err == nil {
				if state.PHPVersion != cfg.PHP.Version || state.Runtime != cfg.PHP.Runtime {
					return fmt.Errorf("PHP version or runtime changed since last build — run frank generate && frank up --build")
				}
			}
		}
	}

	if err := client.Up(composeArgs...); err != nil {
		return err
	}

	if quick {
		return nil
	}

	// Post-start tasks — failures are logged but don't abort.
	if _, err := os.Stat(dir + "/composer.json"); err == nil {
		if err := client.Exec("laravel.test", "composer", "install", "--no-interaction"); err != nil {
			fmt.Fprintf(os.Stderr, "warning: composer install failed: %v\n", err)
		}
	}

	if _, err := os.Stat(dir + "/artisan"); err == nil {
		if err := client.Exec("laravel.test", "php", "artisan", "migrate", "--force"); err != nil {
			fmt.Fprintf(os.Stderr, "warning: artisan migrate failed: %v\n", err)
		}
	}

	return nil
}
