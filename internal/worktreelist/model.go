package worktreelist

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// PostQuitAction describes what to run after the TUI exits.
type PostQuitAction struct {
	Kind string // "logs", "editor"
	Path string
}

// shared holds state that the ItemDelegate needs to read during Render.
// Allocated on the heap so pointers survive bubbletea's value-copy of Model.
type shared struct {
	busyIdx      int
	spinnerFrame int
}

// Model is the root bubbletea model for frank worktree list.
type Model struct {
	list          list.Model
	dir           string
	confirmRemove bool
	statusMsg     string
	postQuit      *PostQuitAction
	quitting      bool
	shared        *shared
}

type actionDoneMsg struct {
	err error
}

type refreshMsg struct{}

type spinnerTickMsg struct{}

func spinnerTick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(_ time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

func newKeyBinding(k, help string) key.Binding {
	return key.NewBinding(key.WithKeys(k), key.WithHelp(k, help))
}

// New creates a Model from discovered worktree items.
func New(items []WorktreeItem, dir string) Model {
	s := &shared{busyIdx: -1}
	m := Model{dir: dir, shared: s}

	listItems := make([]list.Item, len(items))
	for i, item := range items {
		listItems[i] = item
	}

	delegate := ItemDelegate{
		BusyIdx:      &s.busyIdx,
		SpinnerFrame: &s.spinnerFrame,
	}
	l := list.New(listItems, delegate, 80, 24)
	l.Title = "Worktrees"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.Styles.Title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13"))

	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			newKeyBinding("o", "open"),
			newKeyBinding("u", "up"),
			newKeyBinding("d", "down"),
			newKeyBinding("r", "remove"),
			newKeyBinding("l", "logs"),
			newKeyBinding("g", "generate"),
			newKeyBinding("e", "editor"),
		}
	}

	m.list = l
	return m
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		if m.list.FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			return m, cmd
		}

		if m.confirmRemove {
			return m.handleConfirmKey(msg)
		}
		return m.handleKey(msg)

	case actionDoneMsg:
		m.shared.busyIdx = -1
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("error: %v", msg.err)
		} else {
			m.statusMsg = "done"
		}
		return m, m.refresh()

	case refreshMsg:
		return m.doRefresh()

	case spinnerTickMsg:
		if m.shared.busyIdx >= 0 {
			m.shared.spinnerFrame++
			return m, spinnerTick()
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.shared.busyIdx >= 0 {
		return m, nil
	}

	item, ok := m.selectedItem()
	if !ok {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "o":
		err := openBrowser(item)
		if err != nil {
			m.statusMsg = fmt.Sprintf("browser: %v", err)
		}
		return m, nil

	case "r":
		m.confirmRemove = true
		m.statusMsg = fmt.Sprintf("remove %s? (y/n)", item.Branch)
		return m, nil

	case "u":
		m.shared.busyIdx = m.list.Index()
		m.statusMsg = "starting containers..."
		return m, tea.Batch(m.runAction(func() error {
			return upContainers(item.Path)
		}), spinnerTick())

	case "d":
		m.shared.busyIdx = m.list.Index()
		m.statusMsg = "stopping containers..."
		return m, tea.Batch(m.runAction(func() error {
			return downContainers(item.Path)
		}), spinnerTick())

	case "l":
		m.postQuit = &PostQuitAction{Kind: "logs", Path: item.Path}
		m.quitting = true
		return m, tea.Quit

	case "g":
		m.shared.busyIdx = m.list.Index()
		m.statusMsg = "regenerating..."
		return m, tea.Batch(m.runAction(func() error {
			return regenerate(item.Path)
		}), spinnerTick())

	case "e":
		m.postQuit = &PostQuitAction{Kind: "editor", Path: item.Path}
		m.quitting = true
		return m, tea.Quit
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.confirmRemove = false
		item, ok := m.selectedItem()
		if !ok {
			return m, nil
		}
		m.shared.busyIdx = m.list.Index()
		m.statusMsg = "removing worktree..."
		return m, tea.Batch(m.runAction(func() error {
			return removeWorktree(item.Path, item.Branch)
		}), spinnerTick())

	default:
		m.confirmRemove = false
		m.statusMsg = ""
		return m, nil
	}
}

func (m Model) runAction(fn func() error) tea.Cmd {
	return func() tea.Msg {
		return actionDoneMsg{err: fn()}
	}
}

func (m Model) refresh() tea.Cmd {
	return func() tea.Msg {
		return refreshMsg{}
	}
}

func (m Model) doRefresh() (tea.Model, tea.Cmd) {
	items, err := Discover(m.dir)
	if err != nil {
		m.statusMsg = fmt.Sprintf("refresh: %v", err)
		return m, nil
	}

	listItems := make([]list.Item, len(items))
	for i, item := range items {
		listItems[i] = item
	}
	m.list.SetItems(listItems)
	return m, nil
}

func (m Model) selectedItem() (WorktreeItem, bool) {
	item := m.list.SelectedItem()
	if item == nil {
		return WorktreeItem{}, false
	}
	wt, ok := item.(WorktreeItem)
	return wt, ok
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if m.statusMsg != "" {
		m.list.NewStatusMessage(m.statusMsg)
		m.statusMsg = ""
	}
	return m.list.View()
}

func (m Model) PostQuit() *PostQuitAction {
	return m.postQuit
}

func Run(dir string, items []WorktreeItem) error {
	m := New(items, dir)
	p := tea.NewProgram(m, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return err
	}

	final, ok := finalModel.(Model)
	if !ok {
		return nil
	}

	pq := final.PostQuit()
	if pq == nil {
		return nil
	}

	switch pq.Kind {
	case "logs":
		return tailLogs(pq.Path)
	case "editor":
		return openEditor(pq.Path)
	}
	return nil
}
