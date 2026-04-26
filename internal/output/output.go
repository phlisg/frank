package output

import (
	"fmt"
	"os"
	"sync"
	"time"
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

// ANSI color helpers.
const (
	ansiReset  = "\033[0m"
	ansiGreen  = "\033[32m"
	ansiRed    = "\033[31m"
	ansiYellow = "\033[33m"
	ansiClear  = "\r\033[K"
)

// braille spinner frames, 100ms per frame.
var spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Group prints a phase completion tick. Shown in Normal + Verbose.
// Format: "✓ label (detail)" or "✓ label" when detail is empty.
// Use for instant completions; prefer Spin for long-running operations.
func Group(label, detail string) {
	if current == Quiet {
		return
	}
	if detail == "" {
		fmt.Printf("%s✓%s %s\n", ansiGreen, ansiReset, label)
	} else {
		fmt.Printf("%s✓%s %s (%s)\n", ansiGreen, ansiReset, label, detail)
	}
}

// Spin starts a braille spinner with the given label. Returns a stop function.
// Call stop(nil) for green ✓, stop(err) for red ✗. Safe to call stop multiple times.
// In Quiet mode returns a no-op stop function.
func Spin(label string) func(err error) {
	if current == Quiet {
		return func(error) {}
	}

	var once sync.Once
	done := make(chan struct{})

	go func() {
		i := 0
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		// Print initial frame immediately.
		fmt.Printf("%s%s%s %s", ansiYellow, spinFrames[0], ansiReset, label)
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				i = (i + 1) % len(spinFrames)
				fmt.Printf("%s%s%s%s %s", ansiClear, ansiYellow, spinFrames[i], ansiReset, label)
			}
		}
	}()

	return func(err error) {
		once.Do(func() {
			close(done)
			if err != nil {
				fmt.Printf("%s%s✗%s %s\n", ansiClear, ansiRed, ansiReset, label)
			} else {
				fmt.Printf("%s%s✓%s %s\n", ansiClear, ansiGreen, ansiReset, label)
			}
		})
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
