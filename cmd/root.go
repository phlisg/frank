package cmd

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/docker"
	"github.com/phlisg/frank/internal/output"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "frank",
	Short:         "Frank — Laravel Development Environment",
	RunE:          runRoot,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Dir is the global --dir flag value (target project directory).
var Dir string

var flagVerbose bool
var flagQuiet bool

// TemplateFS holds the embedded templates FS passed from main.
var TemplateFS fs.FS

func init() {
	rootCmd.PersistentFlags().StringVar(&Dir, "dir", "", "target directory (defaults to current working directory)")
	rootCmd.RegisterFlagCompletionFunc("dir", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveFilterDirs
	})
	rootCmd.PersistentFlags().BoolVar(&flagVerbose, "verbose", false, "Show detailed output")
	rootCmd.PersistentFlags().BoolVarP(&flagQuiet, "quiet", "q", false, "Suppress all output")
	rootCmd.MarkFlagsMutuallyExclusive("verbose", "quiet")
}

func Execute(fsys fs.FS, version string) {
	TemplateFS = fsys
	rootCmd.Version = version

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		switch {
		case flagVerbose:
			output.SetLevel(output.Verbose)
		case flagQuiet:
			output.SetLevel(output.Quiet)
		default:
			output.SetLevel(output.Normal)
		}
		return nil
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func runRoot(cmd *cobra.Command, args []string) error {
	dir := resolveDir()

	cfg, err := config.Load(dir)
	if err != nil {
		fmt.Println("No frank.yaml found")
		fmt.Println()
		fmt.Println("  Run frank new or frank setup to get started.")
		fmt.Println()
		printCommands(cmd)
		return nil
	}

	projectName := config.ProjectName(dir)
	state, running, total := docker.New(dir).ContainerStatus()

	fmt.Printf("%-12s %s · PHP %s\n", projectName, cfg.PHP.Runtime, cfg.PHP.Version)
	fmt.Printf("%-12s %s\n", "Services", strings.Join(cfg.Services, ", "))

	switch state {
	case docker.StateRunning, docker.StatePartial:
		fmt.Printf("%-12s %d/%d running\n", "Status", running, total)
		fmt.Println()
		fmt.Println("  frank compose ps   view running services")
		fmt.Println("  frank down   stop containers")
	default:
		fmt.Printf("%-12s stopped\n", "Status")
		fmt.Println()
		fmt.Println("  frank up     start containers")
	}

	fmt.Println()
	printCommands(cmd)
	return nil
}

func printCommands(cmd *cobra.Command) {
	fmt.Println("Available commands:")
	for _, sub := range cmd.Commands() {
		if !sub.Hidden {
			fmt.Printf("  %-14s %s\n", sub.Name(), sub.Short)
		}
	}
}

// resolveDir returns Dir if set, otherwise the current working directory.
func resolveDir() string {
	if Dir != "" {
		return Dir
	}
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return dir
}

// writeFile writes content to path, creating or truncating the file.
func writeFile(path, content string) error {
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	return nil
}
