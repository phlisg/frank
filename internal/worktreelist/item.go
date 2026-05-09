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

// Ensure WorktreeItem satisfies list.Item.
var _ list.Item = WorktreeItem{}

// FilterValue implements list.Item. Returns the branch name for filtering.
func (w WorktreeItem) FilterValue() string { return w.Branch }

// Status indicator symbols.
const (
	indicatorRunning       = "●"
	indicatorPartial       = "◐"
	indicatorStopped       = "○"
	indicatorNotConfigured = "✗"
)

// Lipgloss styles for the item delegate.
var (
	itemTitle        = lipgloss.NewStyle().Bold(true)
	itemTitleSelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13"))
	itemDesc         = lipgloss.NewStyle().Faint(true)
	itemDescSelected = lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("13"))
	itemPath         = lipgloss.NewStyle().Faint(true)
	itemPathSelected = lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("13"))

	indicatorRunningStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))  // green
	indicatorPartialStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))  // yellow
	indicatorStoppedStyle       = lipgloss.NewStyle().Faint(true)                      // dim/gray
	indicatorNotConfiguredStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))  // red
)

// ItemDelegate renders WorktreeItem entries as 3-line blocks with 1-line spacing.
type ItemDelegate struct{}

// Ensure ItemDelegate satisfies list.ItemDelegate.
var _ list.ItemDelegate = ItemDelegate{}

// Height returns the item height (3 visible lines).
func (d ItemDelegate) Height() int { return 3 }

// Spacing returns the gap between items (1 blank line).
func (d ItemDelegate) Spacing() int { return 1 }

// Update is a no-op — items are read-only.
func (d ItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

// Render writes the 3-line item view to w.
func (d ItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	wt, ok := item.(WorktreeItem)
	if !ok {
		return
	}

	selected := index == m.Index()

	// Line 1: branch name
	title := wt.Branch
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
	descParts = append(descParts, indicatorStyle.Render(indicator))
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

	fmt.Fprintf(w, "  %s\n  %s\n  %s", title, descLine, path)
}

// statusIndicator returns the symbol and style for a worktree's status.
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

// shortenHome replaces the user's home directory prefix with ~.
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
