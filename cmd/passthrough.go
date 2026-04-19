package cmd

import "github.com/spf13/cobra"

// splitPassthrough returns the args after a literal `--` separator.
// Cobra normally strips the literal "--" token and reports the dash
// position via ArgsLenAtDash when flag parsing is enabled. With
// DisableFlagParsing=true the token survives, so we fall back to a
// scan. Returns nil when no separator is present.
func splitPassthrough(cmd *cobra.Command, args []string) []string {
	if cmd != nil {
		if dash := cmd.ArgsLenAtDash(); dash >= 0 && dash <= len(args) {
			return args[dash:]
		}
	}
	for i, a := range args {
		if a == "--" {
			return args[i+1:]
		}
	}
	return nil
}

// stripDirFlag extracts "--dir <value>" from args, returning the
// resolved directory (falling back to the global --dir / cwd via
// resolveDir) and the remaining args. Used by subcommands that keep
// DisableFlagParsing=true, where cobra's persistent flag parsing does
// not run.
func stripDirFlag(args []string) (dir string, rest []string) {
	dir = resolveDir()
	for i := 0; i < len(args); i++ {
		if args[i] == "--dir" && i+1 < len(args) {
			dir = args[i+1]
			i++
			continue
		}
		rest = append(rest, args[i])
	}
	return dir, rest
}
