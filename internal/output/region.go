package output

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// regionLines is how many trailing output lines the live region shows.
const regionLines = 6

// LiveRegion is a self-redrawing terminal area: a spinner header plus the last
// N lines of streamed output, dimmed. It implements io.Writer — point a
// command's Stdout/Stderr at it. Call Stop to collapse the region to a single
// ✓/✗ tick. It mirrors Spin but for noisy long-running commands (docker build/up).
//
// Behaviour by level / terminal:
//   - Quiet         → discards everything, Stop is silent.
//   - Verbose       → streams raw output through, Stop prints a tick.
//   - Normal, no TTY → discards output, Stop prints a tick (like RunQuiet+Group).
//   - Normal, TTY    → the live region.
type LiveRegion struct {
	label string
	mode  regionMode

	// live-mode state
	mu       sync.Mutex
	partial  []byte
	lines    chan string
	done     chan struct{}
	finished chan struct{}
}

type regionMode int

const (
	regionDiscard regionMode = iota
	regionPassthrough
	regionTick
	regionLive
)

// Region starts a live progress region with the given header label.
func Region(label string) *LiveRegion {
	r := &LiveRegion{label: label}

	switch {
	case current == Quiet:
		r.mode = regionDiscard
	case current == Verbose:
		r.mode = regionPassthrough
	case !term.IsTerminal(int(os.Stdout.Fd())):
		r.mode = regionTick
	default:
		r.mode = regionLive
		r.lines = make(chan string, 128)
		r.done = make(chan struct{})
		r.finished = make(chan struct{})
		go r.render()
	}
	return r
}

// Write implements io.Writer. Safe for concurrent callers (stdout + stderr).
func (r *LiveRegion) Write(p []byte) (int, error) {
	switch r.mode {
	case regionPassthrough:
		return os.Stdout.Write(p)
	case regionDiscard, regionTick:
		return len(p), nil
	}

	// regionLive: accumulate, split on newlines, forward whole lines.
	r.mu.Lock()
	r.partial = append(r.partial, p...)
	for {
		i := bytes.IndexByte(r.partial, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimRight(string(r.partial[:i]), "\r")
		r.partial = r.partial[i+1:]
		select {
		case r.lines <- line:
		case <-r.done:
		}
	}
	r.mu.Unlock()
	return len(p), nil
}

// Stop collapses the region. err nil → green ✓, non-nil → red ✗.
func (r *LiveRegion) Stop(err error) {
	switch r.mode {
	case regionDiscard:
		return
	case regionLive:
		close(r.done)
		<-r.finished
	}
	if err != nil {
		fmt.Printf("%s✗%s %s\n", ansiRed, ansiReset, r.label)
	} else {
		fmt.Printf("%s✓%s %s\n", ansiGreen, ansiReset, r.label)
	}
}

// render owns all terminal drawing for live mode (single goroutine).
func (r *LiveRegion) render() {
	defer close(r.finished)

	ring := make([]string, 0, regionLines)
	prevRows := 0
	frame := 0
	body := lipgloss.NewStyle().Faint(true)

	draw := func() {
		width := regionWidth()
		var b strings.Builder
		if prevRows > 0 {
			fmt.Fprintf(&b, "\033[%dA", prevRows) // up to region top
		}
		b.WriteString("\r\033[2K")
		fmt.Fprintf(&b, "%s%s%s %s\n", ansiYellow, spinFrames[frame], ansiReset, r.label)
		for _, ln := range ring {
			b.WriteString("\033[2K")
			b.WriteString(body.MaxWidth(width).Render("  │ " + ln))
			b.WriteByte('\n')
		}
		b.WriteString("\033[J") // wipe any leftover rows below
		prevRows = 1 + len(ring)
		fmt.Print(b.String())
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	draw()
	for {
		select {
		case <-ticker.C:
			frame = (frame + 1) % len(spinFrames)
			draw()
		case ln := <-r.lines:
			ring = append(ring, ln)
			if len(ring) > regionLines {
				ring = ring[len(ring)-regionLines:]
			}
			draw()
		case <-r.done:
			if prevRows > 0 {
				fmt.Printf("\033[%dA\r\033[J", prevRows) // erase whole region
			}
			return
		}
	}
}

func regionWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return 80
	}
	return w
}
