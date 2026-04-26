package cmd

import (
	"fmt"
	"strings"

	"github.com/phlisg/frank/internal/docker"
	"github.com/phlisg/frank/internal/output"
	"github.com/spf13/cobra"
)

var (
	logLines    string
	logNoFollow bool
)

func init() {
	logCmd.Flags().StringVarP(&logLines, "lines", "n", "25", "Number of lines to show")
	logCmd.Flags().BoolVar(&logNoFollow, "no-follow", false, "Print lines and exit (don't follow)")
	logCmd.AddCommand(logResetCmd)
	rootCmd.AddCommand(logCmd)
}

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Tail the Laravel log",
	Long: `Tail storage/logs/laravel.log inside the app container.

By default, follows the log (like tail -f). Use --no-follow to print and exit.

Examples:
  frank log              # tail -f with last 25 lines
  frank log -n 50        # last 50 lines then follow
  frank log --no-follow  # print last 25 lines and exit`,
	SilenceUsage:      true,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE:              runLog,
}

var logResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Truncate laravel.log to zero bytes",
	Long: `Truncate storage/logs/laravel.log inside the app container.

Example:
  frank log reset`,
	SilenceUsage:      true,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE:              runLogReset,
}

func runLog(cmd *cobra.Command, args []string) error {
	dir := resolveDir()
	client := docker.New(dir)

	if _, err := client.ExecQuiet("laravel.test", "test", "-f", "storage/logs/laravel.log"); err != nil {
		fmt.Println("No logs yet.")
		return nil
	}

	tailArgs := []string{"tail", "-n", logLines}
	if !logNoFollow {
		tailArgs = append(tailArgs, "-f")
	}
	tailArgs = append(tailArgs, "storage/logs/laravel.log")

	err := client.Exec("laravel.test", tailArgs...)
	if err != nil && isInterrupt(err) {
		return nil
	}
	return err
}

func runLogReset(cmd *cobra.Command, args []string) error {
	dir := resolveDir()

	err := docker.New(dir).Exec("laravel.test", "truncate", "-s", "0", "storage/logs/laravel.log")
	if err != nil {
		return err
	}

	output.Group("Reset laravel.log", "")
	return nil
}

func isInterrupt(err error) bool {
	return err != nil && strings.Contains(err.Error(), "code 130")
}
