package cmd

import (
	"github.com/phlisg/frank/internal/docker"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(composeCmd)
}

var composeCmd = &cobra.Command{
	Use:   "compose [args...]",
	Short: "Pass commands directly to docker compose",
	Long: `Pass all arguments directly to docker compose, running in the frank project directory.

Frank-specific flag:
  --dir <path>   Run from the specified directory instead of the project directory

Examples:
  frank compose build --no-cache
  frank compose logs -f laravel.test
  frank compose run --rm laravel.test bash
  frank compose --dir /tmp/frank-test build`,
	DisableFlagParsing: true,
	SilenceUsage:       true,
	RunE:               runCompose,
}

func runCompose(cmd *cobra.Command, args []string) error {
	// Handle help flags before anything else.
	for _, a := range args {
		if a == "-h" || a == "--help" {
			return cmd.Help()
		}
	}

	// Manually extract --dir <value> since DisableFlagParsing=true.
	dir := resolveDir()
	var composeArgs []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--dir" {
			if i+1 < len(args) {
				dir = args[i+1]
				i++ // skip the value
			}
		} else {
			composeArgs = append(composeArgs, args[i])
		}
	}

	return docker.New(dir).Run(composeArgs...)
}
