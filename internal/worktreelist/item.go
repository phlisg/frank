package worktreelist

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var _ list.Item = WorktreeItem{}

func (w WorktreeItem) FilterValue() string { return w.Branch }

const (
	indicatorRunning       = "●"
	indicatorPartial       = "◐"
	indicatorStopped       = "○"
	indicatorNotConfigured = "✗"
)

var (
	itemTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#9086a6"))
	itemDesc  = lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("#9086a6"))
	itemPath  = lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("#9086a6"))

	itemTitleSelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7d68f2"))
	itemDescSelected  = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	itemPathSelected  = lipgloss.NewStyle().Foreground(lipgloss.Color("#7d68f2"))

	indicatorRunningStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	indicatorPartialStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
	indicatorStoppedStyle       = lipgloss.NewStyle().Faint(true)                     // dim/gray
	indicatorNotConfiguredStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red

	busyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// ItemDelegate renders WorktreeItem entries as 3-line blocks with 1-line spacing.
type ItemDelegate struct {
	BusyIdx      *int
	SpinnerFrame *int
}

var _ list.ItemDelegate = ItemDelegate{}

func (d ItemDelegate) Height() int                             { return 3 }
func (d ItemDelegate) Spacing() int                            { return 1 }
func (d ItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d ItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	wt, ok := item.(WorktreeItem)
	if !ok {
		return
	}

	selected := index == m.Index()
	busy := d.BusyIdx != nil && *d.BusyIdx == index

	// Line 1: branch name
	title := wt.Branch

	if busy {
		frame := spinnerFrames[0]
		if d.SpinnerFrame != nil {
			frame = spinnerFrames[*d.SpinnerFrame%len(spinnerFrames)]
		}

		title = busyStyle.Render(frame) + " " + title
	}

	if selected {
		title = itemTitleSelected.Render(title)
	} else {
		title = itemTitle.Render(title)
	}

	// Line 2: status indicator + label + ports
	indicator, indicatorStyle := statusIndicator(wt)
	label := wt.StatusLabel()
	ports := wt.PortSummary()

	var descParts []string
	if !busy {
		descParts = append(descParts, indicatorStyle.Render(indicator))
	}

	if selected {
		descParts = append(descParts, itemDescSelected.Render(label))
	} else {
		descParts = append(descParts, itemDesc.Render(label))
	}

	if ports != "" {
		if selected {
			descParts = append(descParts, itemDescSelected.Render("— "+ports))
		} else {
			descParts = append(descParts, itemDesc.Render("— "+ports))
		}
	}

	descLine := strings.Join(descParts, " ")

	// Line 3: shortened path
	path := shortenHome(wt.Path)
	if selected {
		path = itemPathSelected.Render(path)
	} else {
		path = itemPath.Render(path)
	}

	prefix := "  "
	if selected {
		prefix = "→ "
	}

	fmt.Fprintf(w, "%s%s\n%s\n%s", prefix, title, descLine, path)
}

func statusIndicator(wt WorktreeItem) (string, lipgloss.Style) {
	if !wt.HasFrank {
		return indicatorNotConfigured, indicatorNotConfiguredStyle
	}

	if len(wt.Services) == 0 {
		return indicatorStopped, indicatorStoppedStyle
	}

	running := 0

	for _, s := range wt.Services {
		if s.State == "running" {
			running++
		}
	}

	if running == 0 {
		return indicatorStopped, indicatorStoppedStyle
	}

	if running == len(wt.Services) {
		return indicatorRunning, indicatorRunningStyle
	}

	return indicatorPartial, indicatorPartialStyle
}

func shortenHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}

	if path == home {
		return "~"
	}

	if strings.HasPrefix(path, home+"/") {
		return "~" + path[len(home):]
	}

	return path
}
