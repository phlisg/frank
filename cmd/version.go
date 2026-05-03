package cmd

import (
	"fmt"
	"os"

	selfupdate "github.com/phlisg/frank/internal/update"
	"github.com/spf13/cobra"
)

var (
	versionCheck  bool
	versionUpdate bool
)

func init() {
	versionCmd.Flags().BoolVar(&versionCheck, "check", false, "Check for available updates")
	versionCmd.Flags().BoolVar(&versionUpdate, "update", false, "Update frank to the latest version")
	versionCmd.MarkFlagsMutuallyExclusive("check", "update")
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:               "version",
	Short:             "Print the frank version",
	SilenceUsage:      true,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(cmd *cobra.Command, args []string) error {
		switch {
		case versionCheck:
			status, err := selfupdate.Check(rootCmd.Version)
			if err != nil {
				fmt.Printf("frank is up to date (v%s)\n", rootCmd.Version)
				return nil
			}
			if status.Available {
				fmt.Printf("Update available: v%s (run frank version --update)\n", status.Latest)
			} else {
				fmt.Printf("frank is up to date (v%s)\n", rootCmd.Version)
			}

		case versionUpdate:
			status, err := selfupdate.Check(rootCmd.Version)
			if err != nil || !status.Available {
				fmt.Printf("frank is up to date (v%s)\n", rootCmd.Version)
				return nil
			}
			if err := selfupdate.Run(status.Latest); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

		default:
			fmt.Println(rootCmd.Version)
			status, err := selfupdate.Check(rootCmd.Version)
			if err == nil && status.Available {
				fmt.Printf("\nUpdate available: v%s (run frank version --update)\n", status.Latest)
			}
		}

		return nil
	},
}
