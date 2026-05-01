package cmd

import (
	"fmt"

	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/docker"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(testCmd)
}

var testCmd = &cobra.Command{
	Use:   "test [-- <artisan/pest flags>]",
	Short: "Run tests inside the Laravel container",
	Long: `Executes php artisan test inside the laravel.test container.
Pass artisan/pest flags after --.

Examples:
  frank test                          # run all tests
  frank test -- --parallel            # pest parallel
  frank test -- --filter=SeoAction    # filter tests
  frank test -- --parallel --processes=4

Note: if you see "database testing does not exist", you may need to recreate
your database volume so the init script runs:

  frank down -v
  frank up`,
	SilenceUsage:      true,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE:              runTest,
}

func runTest(cmd *cobra.Command, args []string) error {
	dir := resolveDir()

	if _, err := config.Load(dir); err != nil {
		return err
	}

	client := docker.New(dir)
	state, _, _ := client.ContainerStatus()
	if state != docker.StateRunning && state != docker.StatePartial {
		return fmt.Errorf("containers are not running — run frank up first")
	}

	execArgs := []string{"php", "artisan", "test"}
	if pt := splitPassthrough(cmd, args); len(pt) > 0 {
		execArgs = append(execArgs, pt...)
	}

	return client.Exec("laravel.test", execArgs...)
}
