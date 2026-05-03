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
	selfupdate "github.com/phlisg/frank/internal/update"
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
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		switch {
		case flagVerbose:
			output.SetLevel(output.Verbose)
		case flagQuiet:
			output.SetLevel(output.Quiet)
		default:
			output.SetLevel(output.Normal)
		}
		name := cmd.Name()
		if name == "up" || name == "setup" || name == "frank" {
			if status, err := selfupdate.Check(rootCmd.Version); err == nil && status.Available {
				fmt.Fprintf(os.Stderr, "Update available: v%s (run frank version --update)\n", status.Latest)
			}
		}

		return nil
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

// ANSI escape helpers — no external deps.
const (
	ansiReset   = "\033[0m"
	ansiBold    = "\033[1m"
	ansiDim     = "\033[2m"
	ansiGreen   = "\033[32m"
	ansiRed     = "\033[31m"
)

func runRoot(cmd *cobra.Command, args []string) error {
	dir := resolveDir()

	cfg, err := config.Load(dir)
	if err != nil {
		// No project — show branded header + cobra help.
		fmt.Println()
		fmt.Printf("  %sFrank%s — Laravel Development Environment\n", ansiBold, ansiReset)
		fmt.Println()
		fmt.Printf("  Run %sfrank new%s or %sfrank setup%s to get started.\n", ansiBold, ansiReset, ansiBold, ansiReset)
		fmt.Println()
		printCommands(cmd)
		return nil
	}

	projectName := config.ProjectName(dir)
	state, running, total := docker.New(dir).ContainerStatus()

	// Header
	fmt.Println()
	fmt.Printf("  %sFrank%s — %s%s%s\n", ansiBold, ansiReset, ansiBold, projectName, ansiReset)
	fmt.Println()

	// Info rows
	printRow("PHP", fmt.Sprintf("%s (%s)", cfg.PHP.Version, cfg.PHP.Runtime))
	printRow("Laravel", cfg.Laravel.Version)
	printRow("Services", strings.Join(cfg.Services, ", "))
	printRow("Node", cfg.Node.PackageManager)

	// Workers summary
	if cfg.Workers.Schedule || len(cfg.Workers.Queue) > 0 {
		var parts []string
		if cfg.Workers.Schedule {
			dot := colorDot(state)
			parts = append(parts, dot+" scheduler")
		}
		if len(cfg.Workers.Queue) > 0 {
			queueTotal := 0
			for _, p := range cfg.Workers.Queue {
				queueTotal += p.Count
			}
			parts = append(parts, fmt.Sprintf("%d× queue", queueTotal))
		}
		printRow("Workers", strings.Join(parts, "  "))
	}

	// Status
	switch state {
	case docker.StateRunning:
		printRow("Status", fmt.Sprintf("%s%s●%s %d/%d running", ansiGreen, ansiBold, ansiReset, running, total))
	case docker.StatePartial:
		printRow("Status", fmt.Sprintf("%s%s●%s %d/%d running", ansiRed, ansiBold, ansiReset, running, total))
	default:
		printRow("Status", fmt.Sprintf("%s%s●%s stopped", ansiRed, ansiBold, ansiReset))
	}

	// Next-step hints
	fmt.Println()
	switch state {
	case docker.StateRunning, docker.StatePartial:
		printHint("frank compose ps", "view running services")
		printHint("frank down", "stop containers")
	default:
		printHint("frank up", "start containers")
	}
	fmt.Println()

	return nil
}

// colorDot returns a green or red bullet based on container state.
func colorDot(state docker.ContainerState) string {
	if state == docker.StateRunning {
		return ansiGreen + ansiBold + "●" + ansiReset
	}
	return ansiRed + ansiBold + "●" + ansiReset
}

// printRow prints a dim label + value, indented.
func printRow(label, value string) {
	fmt.Printf("  %s%-12s%s%s\n", ansiDim, label, ansiReset, value)
}

// printHint prints a command hint line.
func printHint(command, desc string) {
	fmt.Printf("  %-22s%s%s%s\n", command, ansiDim, desc, ansiReset)
}

func printCommands(cmd *cobra.Command) {
	mainNames := []string{"new", "up", "down", "install", "setup"}
	mainSet := make(map[string]bool, len(mainNames))
	for _, n := range mainNames {
		mainSet[n] = true
	}

	subs := make(map[string]*cobra.Command)
	var otherNames []string
	for _, sub := range cmd.Commands() {
		if sub.Hidden {
			continue
		}
		subs[sub.Name()] = sub
		if !mainSet[sub.Name()] {
			otherNames = append(otherNames, sub.Name())
		}
	}

	fmt.Printf("  %sMain Commands:%s\n", ansiDim, ansiReset)
	for _, name := range mainNames {
		if sub, ok := subs[name]; ok {
			fmt.Printf("    %-14s%s%s%s\n", name, ansiDim, sub.Short, ansiReset)
		}
	}
	fmt.Println()

	fmt.Printf("  %sOther Commands:%s\n", ansiDim, ansiReset)
	for _, name := range otherNames {
		sub := subs[name]
		fmt.Printf("    %-14s%s%s%s\n", name, ansiDim, sub.Short, ansiReset)
	}
	fmt.Println()
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
