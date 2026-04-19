// Package workertop implements the `frank worker top` TUI.
//
// This file owns Pane: the bubbletea model for a single worker's pane,
// composed of a title bar (name · mem · exit badge) and a scrollable
// viewport holding the last PaneBufferCap log lines.
//
// Panes are self-contained — TopModel translates raw stream events
// (docker logs, stats hub, etc.) into PaneID-addressed messages and
// forwards them. Each Pane ignores messages whose PaneID doesn't match
// its own spec.
package workertop

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// PaneBufferCap is the per-pane FIFO line limit. Older lines are
// dropped as new ones arrive.
const PaneBufferCap = 500

// degradedAfter is how long without a stats sample (while still
// StateRunning) before the pane's border flips from green to yellow.
const degradedAfter = 10 * time.Second

// Pane is a bubbletea model rendering one worker's title bar + viewport.
type Pane struct {
	spec     PaneSpec
	viewport viewport.Model
	stats    StatsSample
	statsAge time.Time
	state    PaneState
	exitCode int
	focused  bool
	width    int
	height   int
	// truncateTitle is set by layout messages; triggers shortName()
	// rendering in the title bar when the pane is too narrow for the
	// full name.
	truncateTitle bool

	// buffer is a FIFO ring holding up to PaneBufferCap lines. Append
	// only; trimmed from the front when full.
	buffer []string
}

// Pane messages. TopModel translates raw events into these
// PaneID-addressed envelopes and forwards every one to every pane;
// each pane filters on its own spec.Name.

// LogLineMsg delivers one log line to the pane identified by PaneID.
type LogLineMsg struct {
	PaneID string
	Line   string
}

// StatsMsg is a broadcast snapshot of the current stats-hub state;
// each pane picks its own ContainerID entry out of the map.
type StatsMsg map[string]StatsSample

// ResizeMsg tells the pane its new dimensions and whether the title
// should be compressed to its short form.
type ResizeMsg struct {
	PaneID        string
	Width, Height int
	TruncateTitle bool
}

// FocusMsg flips the pane's focused bool — drives the cyan border
// override in View.
type FocusMsg struct {
	PaneID  string
	Focused bool
}

// StateMsg notifies the pane of a state transition — e.g. logs reader
// hit EOF → StateExited. ExitCode is meaningful only when
// State == StateExited.
type StateMsg struct {
	PaneID   string
	State    PaneState
	ExitCode int
}

// NewPane constructs a Pane pre-populated with the spec's initial
// state and an empty viewport. Width/Height are zero until the first
// ResizeMsg lands.
func NewPane(spec PaneSpec) *Pane {
	vp := viewport.New(0, 0)
	return &Pane{
		spec:     spec,
		viewport: vp,
		state:    spec.State,
		exitCode: spec.ExitCode,
	}
}

// Init satisfies tea.Model. The pane has no deferred setup — all
// streams (logs, stats) are driven from TopModel and arrive as
// PaneID-addressed messages.
func (p *Pane) Init() tea.Cmd {
	return nil
}

// Update satisfies tea.Model. Only messages whose PaneID matches the
// pane's spec.Name take effect; StatsMsg is a broadcast handled by
// ContainerID lookup. Anything else returns the pane unchanged.
func (p *Pane) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case LogLineMsg:
		if m.PaneID != p.spec.Name {
			return p, nil
		}
		p.appendLine(m.Line)
		return p, nil

	case StatsMsg:
		if p.spec.ContainerID == "" {
			return p, nil
		}
		if sample, ok := m[p.spec.ContainerID]; ok {
			p.stats = sample
			p.statsAge = time.Now()
		}
		return p, nil

	case ResizeMsg:
		if m.PaneID != p.spec.Name {
			return p, nil
		}
		p.width = m.Width
		p.height = m.Height
		p.truncateTitle = m.TruncateTitle
		// Reserve 2 cols/rows for the border and 1 row for the title
		// bar. Minimum 1×1 so the viewport never goes negative.
		innerW := p.width - 2
		innerH := p.height - 2 - 1
		if innerW < 1 {
			innerW = 1
		}
		if innerH < 1 {
			innerH = 1
		}
		p.viewport.Width = innerW
		p.viewport.Height = innerH
		return p, nil

	case FocusMsg:
		if m.PaneID != p.spec.Name {
			return p, nil
		}
		p.focused = m.Focused
		return p, nil

	case StateMsg:
		if m.PaneID != p.spec.Name {
			return p, nil
		}
		p.state = m.State
		p.exitCode = m.ExitCode
		return p, nil
	}
	return p, nil
}

// View renders title bar + viewport wrapped in the state-colored
// (or focus-colored) border.
func (p *Pane) View() string {
	body := lipgloss.JoinVertical(lipgloss.Left, p.titleBar(), p.viewport.View())

	style := p.borderStyle()
	w := p.width - 2
	h := p.height - 2
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return style.Width(w).Height(h).Render(body)
}

// appendLine pushes a line onto the FIFO, trims the front when the
// cap is exceeded, and pushes the updated content into the viewport
// with auto-scroll to bottom.
func (p *Pane) appendLine(line string) {
	p.buffer = append(p.buffer, line)
	if len(p.buffer) > PaneBufferCap {
		// Drop oldest; slice tail to amortize over many appends.
		drop := len(p.buffer) - PaneBufferCap
		p.buffer = append(p.buffer[:0], p.buffer[drop:]...)
	}
	p.viewport.SetContent(strings.Join(p.buffer, "\n"))
	p.viewport.GotoBottom()
}

// borderStyle picks a border by (focused, state, stats-age). Focus
// overrides state color — the spec calls out a cyan focus border.
func (p *Pane) borderStyle() lipgloss.Style {
	if p.focused {
		return BorderFocused
	}
	switch p.state {
	case StateExited:
		return BorderExited
	case StateRunning:
		// Degraded: running but no stats received for 10s+. Ignore the
		// pre-first-sample window (statsAge zero) — a brand-new pane
		// shouldn't flash yellow before the hub's first tick lands.
		if !p.statsAge.IsZero() && time.Since(p.statsAge) > degradedAfter {
			return BorderDegraded
		}
		return BorderRunning
	default:
		// StateMissing and anything else fall through to the exited
		// style — same semantics from the user's point of view.
		return BorderExited
	}
}

// titleBar renders "name · 42 MB · [exited 137]" trimmed/padded to
// the pane's inner width. Left side is the name (or shortName when
// truncateTitle is set); right side is the mem figure plus the
// exit-code badge when applicable.
func (p *Pane) titleBar() string {
	name := p.spec.Name
	if p.truncateTitle {
		name = p.shortName()
	}
	left := TitleName.Render(name)

	memStr := "—"
	if p.stats.MemBytes > 0 {
		// Round to nearest MB. docker stats reports in MiB internally;
		// display rounds to integer for CCTV-clean readability.
		mb := (p.stats.MemBytes + (1<<20)/2) / (1 << 20)
		memStr = fmt.Sprintf("%d MB", mb)
	}
	right := TitleMem.Render(memStr)

	if p.state == StateExited {
		right = right + " " + TitleExit.Render(fmt.Sprintf("[exited %d]", p.exitCode))
	}

	// Inner width equals viewport width (title + viewport share the
	// same inner region of the border).
	innerW := p.viewport.Width
	if innerW < 1 {
		innerW = 1
	}

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := innerW - leftW - rightW
	if gap < 1 {
		// Not enough room for both halves; prefer showing the name.
		// Clamp to at least one space so the right half doesn't
		// collide with the name.
		return left + " " + right
	}
	return left + strings.Repeat(" ", gap) + right
}

// shortName compresses declared worker names when the pane is too
// narrow for the full form:
//
//	laravel.queue.<pool>.<i> → laravel.q.<pool[:3]>.<i>
//	laravel.schedule         → laravel.sched
//	<adhoc>                  → unchanged (user picks the name)
func (p *Pane) shortName() string {
	switch p.spec.Kind {
	case KindSchedule:
		return "laravel.sched"
	case KindQueue:
		parts := strings.Split(p.spec.Name, ".")
		// Expect: ["laravel", "queue", pool, index]. Fall back to the
		// raw name if the shape is unexpected.
		if len(parts) != 4 || parts[0] != "laravel" || parts[1] != "queue" {
			return p.spec.Name
		}
		pool := parts[2]
		if len(pool) > 3 {
			pool = pool[:3]
		}
		return fmt.Sprintf("laravel.q.%s.%s", pool, parts[3])
	default:
		return p.spec.Name
	}
}
