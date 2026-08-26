package worktreelist

import (
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/phlisg/frank/internal/config"
)

// PostQuitAction describes what to run after the TUI exits.
type PostQuitAction struct {
	Kind string // "logs", "editor"
	Path string
}

// shared holds state that the ItemDelegate needs to read during Render.
// Allocated on the heap so pointers survive bubbletea's value-copy of Model.
type shared struct {
	// busy is keyed by worktree path: several worktrees may run actions at once.
	// Only ever touched from the UI goroutine (Update and Render), so no lock.
	busy         map[string]bool
	spinnerFrame int
	// send delivers messages from action goroutines back into the program.
	// Set by Run once the program exists; nil in tests.
	send func(tea.Msg)
}

// Model is the root bubbletea model for frank worktree list.
type Model struct {
	list          list.Model
	dir           string
	confirmRemove bool
	creating      bool
	createSeed    bool
	branchInput   textinput.Model
	statusMsg     string
	progress      []string
	width         int
	height        int
	postQuit      *PostQuitAction
	quitting      bool
	shared        *shared
}

type actionDoneMsg struct {
	path   string
	branch string
	err    error
}

type refreshMsg struct {
	items []WorktreeItem
	err   error
}

type spinnerTickMsg struct{}

func spinnerTick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(_ time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

func newKeyBinding(k, help string) key.Binding {
	return key.NewBinding(key.WithKeys(k), key.WithHelp(k, help))
}

var nonAlphanumDash = regexp.MustCompile(`[^a-z0-9-]+`)

func BranchToKebab(branch string) string {
	s := strings.ToLower(branch)
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "_", "-")
	s = nonAlphanumDash.ReplaceAllString(s, "")
	s = strings.Trim(s, "-")

	return s
}

// New creates a Model from discovered worktree items.
func New(items []WorktreeItem, dir string) Model {
	s := &shared{busy: map[string]bool{}}

	ti := textinput.New()
	ti.Placeholder = "feature/my-branch"
	ti.CharLimit = 100
	ti.Width = 40

	m := Model{dir: dir, shared: s, branchInput: ti}

	listItems := make([]list.Item, len(items))
	for i, item := range items {
		listItems[i] = item
	}

	delegate := ItemDelegate{
		Busy:         s.busy,
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
			newKeyBinding("c", "create"),
			newKeyBinding("C", "create (empty db)"),
			newKeyBinding("r", "remove"),
			newKeyBinding("l", "logs"),
			newKeyBinding("g", "generate"),
			newKeyBinding("s", "sync db"),
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
		m.width, m.height = msg.Width, msg.Height
		m.applySize()

		return m, nil

	case tea.KeyMsg:
		// Checked before every mode branch: ctrl+c/ctrl+d must escape the filter,
		// the create prompt and a running action alike.
		if k := msg.String(); k == "ctrl+c" || k == "ctrl+d" {
			m.quitting = true
			return m, tea.Quit
		}

		if m.creating {
			return m.handleCreateKey(msg)
		}

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
		delete(m.shared.busy, msg.path)

		// Progress lines belong to whichever actions are still running; clearing
		// them only once the last one finishes keeps the region from flickering.
		if len(m.shared.busy) == 0 {
			m.progress = nil
			m.applySize()
		}

		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("%s: %v", msg.branch, msg.err)
		} else {
			m.statusMsg = msg.branch + ": done"
		}

		return m, m.refresh()

	case refreshMsg:
		return m.applyRefresh(msg)

	case progressMsg:
		// Lines are truncated and the list is shrunk by the region's height, so
		// the total row count never changes — otherwise the view jitters on
		// every line of output.
		m.progress = append(m.progress, truncate(msg.line, m.width-4))
		if len(m.progress) > progressLines {
			m.progress = m.progress[len(m.progress)-progressLines:]
		}

		m.applySize()

		return m, nil

	case spinnerTickMsg:
		if len(m.shared.busy) > 0 {
			m.shared.spinnerFrame++
			return m, spinnerTick()
		}

		return m, nil
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)

	return m, cmd
}

// progressLines is how many trailing output lines the action region shows.
const progressLines = 5

// progressRegion is the running-action block at the bottom of the screen.
// Hex rather than a 256-palette index so it degrades to a visible colour instead
// of plain black on terminals that only report ANSI16.
var progressRegion = lipgloss.NewStyle().
	Background(lipgloss.Color("#2b2440")).
	Foreground(lipgloss.Color("#cfc7e8"))

// applySize re-sizes the list to whatever the progress region leaves over.
func (m *Model) applySize() {
	if m.height == 0 {
		return
	}

	m.list.SetSize(m.width, m.height-m.progressHeight())
}

func (m Model) progressHeight() int {
	if len(m.progress) == 0 {
		return 0
	}

	// Fixed height + the separating newline. Growing the region line by line
	// would shift the list under the cursor on every write.
	return progressLines + 1
}

func truncate(s string, max int) string {
	r := []rune(s)
	if max < 4 || len(r) <= max {
		return s
	}

	return string(r[:max-1]) + "…"
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		m.quitting = true

		return m, tea.Quit

	case "c", "C":
		m.creating = true
		// Lowercase clones the main project's database on first up; uppercase
		// leaves the worktree with an empty one.
		m.createSeed = msg.String() == "c"
		m.branchInput.Reset()
		m.branchInput.Focus()
		m.statusMsg = ""

		return m, m.branchInput.Cursor.BlinkCmd()

	case "o":
		item, ok := m.selectedItem()
		if !ok {
			break
		}

		err := openBrowser(item)
		if err != nil {
			m.statusMsg = fmt.Sprintf("browser: %v", err)
		}

		return m, nil

	case "r":
		item, ok := m.selectedItem()
		if !ok {
			break
		}

		m.confirmRemove = true
		m.statusMsg = fmt.Sprintf("remove %s — deletes branch, containers and database volumes. (y/n)", item.Branch)

		return m, nil

	case "u":
		item, ok := m.selectedItem()
		if !ok || !m.start(item) {
			break
		}

		if needsGenerate(item.Path) {
			m.statusMsg = item.Branch + ": generating + starting containers..."
		} else {
			m.statusMsg = item.Branch + ": starting containers..."
		}

		w := m.progressWriter(item)

		return m, tea.Batch(m.runAction(item, func() error {
			return upContainers(item.Path, w)
		}), spinnerTick())

	case "d":
		item, ok := m.selectedItem()
		if !ok || !m.start(item) {
			break
		}

		m.statusMsg = item.Branch + ": stopping containers..."
		w := m.progressWriter(item)

		return m, tea.Batch(m.runAction(item, func() error {
			return downContainers(item.Path, w)
		}), spinnerTick())

	case "l":
		item, ok := m.selectedItem()
		if !ok {
			break
		}

		m.postQuit = &PostQuitAction{Kind: "logs", Path: item.Path}
		m.quitting = true

		return m, tea.Quit

	case "g":
		item, ok := m.selectedItem()
		if !ok || !m.start(item) {
			break
		}

		m.statusMsg = item.Branch + ": regenerating..."
		w := m.progressWriter(item)

		return m, tea.Batch(m.runAction(item, func() error {
			return regenerate(item.Path, w)
		}), spinnerTick())

	case "s":
		item, ok := m.selectedItem()
		if !ok || !m.start(item) {
			break
		}

		m.statusMsg = item.Branch + ": cloning database from main project..."

		mainDir := m.dir
		w := m.progressWriter(item)

		return m, tea.Batch(m.runAction(item, func() error {
			return CloneDB(item.Path, mainDir, w)
		}), spinnerTick())

	case "e":
		item, ok := m.selectedItem()
		if !ok {
			break
		}

		m.postQuit = &PostQuitAction{Kind: "editor", Path: item.Path}
		m.quitting = true

		return m, tea.Quit
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)

	return m, cmd
}

func (m Model) handleCreateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		branch := strings.TrimSpace(m.branchInput.Value())
		if branch == "" {
			m.creating = false
			m.statusMsg = ""

			return m, nil
		}

		m.creating = false

		projectName := config.ProjectName(m.dir)
		kebab := BranchToKebab(branch)
		parentDir := filepath.Dir(m.dir)
		wtPath := filepath.Join(parentDir, projectName+"-"+kebab)

		seed := m.createSeed
		repoDir := m.dir

		// The worktree does not exist yet, so stand in for it until the refresh
		// picks up the real entry — same path, so the busy key already matches.
		item := WorktreeItem{Path: wtPath, Branch: branch}
		m.start(item)

		m.statusMsg = fmt.Sprintf("creating %s...", kebab)
		w := m.progressWriter(item)

		return m, tea.Batch(m.runAction(item, func() error {
			return CreateWorktree(repoDir, wtPath, branch, seed, w)
		}), spinnerTick())

	case tea.KeyEsc:
		m.creating = false
		m.statusMsg = ""

		return m, nil
	}

	var cmd tea.Cmd
	m.branchInput, cmd = m.branchInput.Update(msg)

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

		if !m.start(item) {
			return m, nil
		}

		m.statusMsg = item.Branch + ": removing worktree..."
		w := m.progressWriter(item)

		return m, tea.Batch(m.runAction(item, func() error {
			return RemoveWorktree(item.Path, item.Branch, w)
		}), spinnerTick())

	default:
		m.confirmRemove = false
		m.statusMsg = ""

		return m, nil
	}
}

// start marks item busy, returning false when an action is already running on it.
// Different worktrees may run in parallel — they are separate compose projects on
// ephemeral ports — but two actions on the same one would fight over one project.
func (m Model) start(item WorktreeItem) bool {
	if m.shared.busy[item.Path] {
		return false
	}

	m.shared.busy[item.Path] = true

	return true
}

// progressWriter returns the io.Writer that streams an action's output into the
// progress region. Actions run off the UI goroutine, so they must not print to
// the terminal directly — that scribbles over the alt-screen. Lines are tagged
// with the branch because several actions can be writing into the region at once.
func (m Model) progressWriter(item WorktreeItem) io.Writer {
	return &lineWriter{send: m.shared.send, prefix: item.Branch + " | "}
}

func (m Model) runAction(item WorktreeItem, fn func() error) tea.Cmd {
	return func() tea.Msg {
		return actionDoneMsg{path: item.Path, branch: item.Branch, err: fn()}
	}
}

// refresh re-discovers worktrees inside the Cmd goroutine. Doing the docker
// probes inline in Update would block the UI for as long as they take.
func (m Model) refresh() tea.Cmd {
	dir := m.dir

	return func() tea.Msg {
		items, err := Discover(dir)
		return refreshMsg{items: items, err: err}
	}
}

func (m Model) applyRefresh(msg refreshMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.statusMsg = fmt.Sprintf("refresh: %v", msg.err)
		return m, nil
	}

	listItems := make([]list.Item, len(msg.items))
	for i, item := range msg.items {
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

	if m.creating {
		label := "  Branch name: "
		if !m.createSeed {
			label = "  Branch name (empty db): "
		}

		return m.list.View() + "\n\n" + label + m.branchInput.View()
	}

	if m.statusMsg != "" {
		m.list.NewStatusMessage(m.statusMsg)
		m.statusMsg = ""
	}

	if len(m.progress) == 0 {
		return m.list.View()
	}

	lines := make([]string, progressLines)
	copy(lines[progressLines-len(m.progress):], m.progress)

	// Width() pads every line to the full terminal width, so the background paints
	// a solid block — the point being that "something is running" reads at a glance
	// even through a translucent terminal.
	return m.list.View() + "\n" + progressRegion.Width(m.width).Render(strings.Join(lines, "\n"))
}

func (m Model) PostQuit() *PostQuitAction {
	return m.postQuit
}

func Run(dir string, items []WorktreeItem) error {
	m := New(items, dir)
	p := tea.NewProgram(m, tea.WithAltScreen())
	m.shared.send = p.Send

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
