package cmd

import (
	"fmt"

	"github.com/phlisg/frank-cli/internal/docker"
	"github.com/spf13/cobra"
)

var quickMode bool

func init() {
	upCmd.Flags().BoolVar(&quickMode, "quick", false, "skip post-start tasks (composer install, artisan migrate)")
	rootCmd.AddCommand(upCmd)
}

var upCmd = &cobra.Command{
	Use:          "up",
	Short:        "Start containers",
	SilenceUsage: true,
	RunE:         runUp,
}

func runUp(cmd *cobra.Command, args []string) error {
	dir := resolveDir()
	client := docker.New(dir)

	if err := client.Up(); err != nil {
		return err
	}

	if quickMode {
		return nil
	}

	// Post-start tasks — failures are logged but don't abort.
	if err := client.Exec("laravel.test", "composer", "install", "--no-interaction"); err != nil {
		fmt.Printf("warning: composer install failed: %v\n", err)
	}

	if err := client.Exec("laravel.test", "php", "artisan", "migrate", "--force"); err != nil {
		fmt.Printf("warning: artisan migrate failed: %v\n", err)
	}

	return nil
}
