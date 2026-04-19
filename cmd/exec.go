package cmd

import (
	"github.com/phlisg/frank/internal/docker"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(execCmd)
}

var execCmd = &cobra.Command{
	Use:   "exec <command> [args...]",
	Short: "Run a command in the laravel.test container as sail",
	Long: `Run a command inside the laravel.test container as the sail user.
Equivalent to: frank compose exec --user sail laravel.test <command> [args...]

Examples:
  frank exec php vendor/bin/pint
  frank exec php vendor/bin/rector process
  frank exec php vendor/bin/phpstan analyse
  frank exec bash`,
	DisableFlagParsing: true,
	SilenceUsage:       true,
	ValidArgsFunction:  cobra.NoFileCompletions,
	RunE:               runExec,
}

func runExec(cmd *cobra.Command, args []string) error {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			return cmd.Help()
		}
	}

	dir, execArgs := stripDirFlag(args)
	return docker.New(dir).Exec("laravel.test", execArgs...)
}
