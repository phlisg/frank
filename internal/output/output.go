package output

import (
	"fmt"
	"os"
)

type Level int

const (
	Quiet Level = iota
	Normal
	Verbose
)

var current = Normal

func SetLevel(l Level) { current = l }
func GetLevel() Level  { return current }

// Group prints a phase completion tick. Shown in Normal + Verbose.
// Format: "✓ label (detail)" or "✓ label" when detail is empty.
func Group(label, detail string) {
	if current == Quiet {
		return
	}
	if detail == "" {
		fmt.Printf("✓ %s\n", label)
	} else {
		fmt.Printf("✓ %s (%s)\n", label, detail)
	}
}

// Detail prints a single operation line. Shown in Verbose only.
func Detail(msg string) {
	if current != Verbose {
		return
	}
	fmt.Printf("  %s\n", msg)
}

// NextSteps prints a "Next steps:" header followed by indented lines.
// Shown in Normal + Verbose. Skips if lines is empty.
func NextSteps(lines []string) {
	if current == Quiet || len(lines) == 0 {
		return
	}
	fmt.Println("\nNext steps:")
	for _, l := range lines {
		fmt.Printf("  %s\n", l)
	}
}

// Warning prints to stderr regardless of level.
func Warning(msg string) {
	fmt.Fprintf(os.Stderr, "warning: %s\n", msg)
}
