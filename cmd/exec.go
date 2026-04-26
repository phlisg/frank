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
	Short: "Run a command inside the app container",
	Long: `Run an arbitrary command inside the laravel.test container as the sail user.
Useful for one-off commands that don't have a shell alias.

Examples:
  frank exec bash                           # interactive shell
  frank exec php vendor/bin/pint            # run Pint code fixer
  frank exec php vendor/bin/rector process  # run Rector refactoring
  frank exec php vendor/bin/phpstan analyse # run static analysis
  frank exec cat .env                       # inspect container env
  frank exec yarn add lodash                # install a JS package`,
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
