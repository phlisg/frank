package cmd

import (
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	selfupdate "github.com/phlisg/frank/internal/update"
	"golang.org/x/term"
)

// offerUpdate checks for a newer frank and, on an interactive terminal, asks
// whether to install it before continuing with the command.
func offerUpdate() {
	if flagQuiet || !term.IsTerminal(int(os.Stdin.Fd())) {
		return
	}

	status, err := selfupdate.Check(rootCmd.Version)
	if err != nil || !status.Available {
		return
	}

	yes := false
	prompt := huh.NewConfirm().
		Title(fmt.Sprintf("Frank %s is available (you have %s). Update now?", status.Latest, rootCmd.Version)).
		Affirmative("Yes").
		Negative("No").
		Value(&yes)

	if err := prompt.Run(); err != nil || !yes {
		return
	}

	if err := selfupdate.Run(status.Latest); err != nil {
		fmt.Fprintln(os.Stderr, "Update failed:", err)
		return
	}

	// ponytail: no re-exec — this run finishes on the old binary, next one is new.
	fmt.Fprintf(os.Stderr, "Updated to %s — continuing with the current version.\n", status.Latest)
}
