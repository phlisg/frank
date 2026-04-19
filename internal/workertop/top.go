// Package workertop implements the `frank worker top` TUI.
//
// This file owns TopModel — the root bubbletea model. TopModel wires the
// header/footer, every row/pane, the stats hub, per-pane log readers, and
// (under --live) the ad-hoc reconciler into a single program; routes keys
// to focus + zoom; and tears every subprocess down cleanly on quit.
//
// See docs/superpowers/specs/2026-04-19-worker-top-tui-design.md for the
// full design — especially the "Model tree", "Shared services", "Data
// Flow", "Layout Algorithm", and "Lifecycle" sections, which this file
// implements end-to-end.
package workertop

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/docker"
)

// Opts controls the behaviour of Run. Fields with unexported names (lower-
// cased) are set by tests; production callers fill only the exported flags
// and let the zero-values wire the real docker-backed implementations.
type Opts struct {
	// Live enables the 2s ad-hoc reconciler. When false, the TUI shows a
	// snapshot of containers at cold start.
	Live bool
	// MinPaneWidth is the minimum column width before pane titles truncate.
	// Zero → defaulted to 30 by the layout engine.
	MinPaneWidth int

	// Test seams — unexported, zero-valued in production.
	statsExec ExecFn
	logsExec  CmdStartFn
	inspector containerInspector
	lister    adhocLister
}

// rowGroup is one logical row in the grid: the schedule row, one per
// declared pool, or the ad-hoc row. paneIDs is ordered and matches the
// visual left-to-right order within the row.
type rowGroup struct {
	kind    PaneKind
	label   string
	pool    string
	paneIDs []string
}

// TopModel is the root bubbletea model. All shared services (stats hub,
// reconciler, per-pane log readers) are owned here and torn down when the
// program exits.
type TopModel struct {
	cfg         *config.Config
	projectName string
	opts        Opts

	// Panes organised into ordered rows.
	panesByID map[string]*Pane
	rowOrder  []rowGroup

	// Layout state — recomputed on every tea.WindowSizeMsg.
	width, height int
	layout        Layout

	// paneBounds maps paneID → absolute terminal rect (0-indexed cells).
	// Populated by recomputeLayout; consumed by mouse hit-testing in Update.
	paneBounds map[string]paneRect

	// Focus / zoom — focusedID == "" means no pane focused;
	// zoomedID != "" means rendering one pane full-screen.
	focusedID string
	zoomedID  string

	// Shared services.
	statsHub    *Hub
	reconciler  *Reconciler // nil when !opts.Live
	logsReaders map[string]*LogsReader
}

// Internal message types. reconcileMsg routes a ReconcileEvent through the
// tea.Msg pipeline; paneCleanupMsg fires after the grace period for
// EventRemove so the pane can finally be deleted.
type reconcileMsg struct {
	evt ReconcileEvent
}

// paneRect is an absolute terminal rect in cells (x/y 0-indexed).
type paneRect struct {
	x, y, w, h int
}

func (r paneRect) contains(x, y int) bool {
	return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h
}
type paneCleanupMsg struct {
	paneID string
}

// Run is the single entry point. It discovers the pane set, starts the
// background services, runs the bubbletea program to completion, and
// reaps every goroutine before returning.
//
// Returns nil on clean user exit (q/Ctrl-C) or empty-fleet exit. Returns
// an error only on setup failure — discovery errors, program-run errors.
func Run(ctx context.Context, cfg *config.Config, projectName string, dc *docker.Client, opts Opts) error {
	// Build the docker adapters if the caller didn't inject test doubles.
	inspector := opts.inspector
	if inspector == nil {
		inspector = &dockerInspector{c: dc}
	}
	lister := opts.lister
	if lister == nil {
		lister = &dockerLister{c: dc, projectName: projectName}
	}

	// 1. Discovery.
	specs, err := discoverWorkers(cfg, projectName, inspector)
	if err != nil {
		return fmt.Errorf("workertop: discover: %w", err)
	}
	if len(specs) == 0 {
		fmt.Fprintln(os.Stderr, "no workers running — declare in frank.yaml or run `frank worker queue`")
		return nil
	}

	// 2. Build row order + panes.
	rows, panes := buildRows(cfg, specs)

	// 3. Stats hub for every pane with a resolved ID.
	var ids []string
	for _, s := range specs {
		if s.ContainerID != "" {
			ids = append(ids, s.ContainerID)
		}
	}
	hub := NewHub(ids, DefaultInterval, opts.statsExec)

	// 4. Per-pane log readers — only for running panes. Missing/exited
	//    panes skip the subprocess entirely (nothing to tail).
	logsExec := opts.logsExec
	if logsExec == nil {
		logsExec = DefaultCmdStartFn
	}
	readers := make(map[string]*LogsReader, len(specs))
	for _, s := range specs {
		if s.State != StateRunning {
			continue
		}
		readers[s.Name] = NewLogsReader(s, logsExec)
	}

	// 5. Reconciler — only under --live.
	var rec *Reconciler
	if opts.Live {
		rec = NewReconciler(lister, DefaultInterval, specs)
	}

	// 6. Build TopModel.
	m := &TopModel{
		cfg:         cfg,
		projectName: projectName,
		opts:        opts,
		panesByID:   panes,
		rowOrder:    rows,
		statsHub:    hub,
		reconciler:  rec,
		logsReaders: readers,
	}
	if firstID := m.firstPaneID(); firstID != "" {
		m.focusedID = firstID
		panes[firstID].focused = true
	}

	// 7. Launch background services under a shared cancellable context.
	//    On program exit we cancel and wait for everything to reap.
	svcCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() { defer wg.Done(); hub.Run(svcCtx) }()

	for _, r := range readers {
		wg.Add(1)
		go func(r *LogsReader) { defer wg.Done(); r.Run(svcCtx) }(r)
	}

	if rec != nil {
		wg.Add(1)
		go func() { defer wg.Done(); rec.Run(svcCtx) }()
	}

	// 8. Run the program. tea.WithContext wires bubbletea's own cancel
	//    path to the same ctx so Ctrl-C arrives as tea.Quit.
	prog := tea.NewProgram(m,
		tea.WithContext(svcCtx),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	_, runErr := prog.Run()

	// 9. Shutdown. Cancel the service context, wait a bounded time for
	//    subprocesses to reap. Anything still alive after the timeout is
	//    abandoned — the process exits regardless.
	cancel()
	waitTimeout(&wg, 3*time.Second)

	if runErr != nil {
		return fmt.Errorf("workertop: bubbletea run: %w", runErr)
	}
	return nil
}

// buildRows groups PaneSpecs into ordered rows + a paneID → *Pane map.
// Schedule first (if any), then one row per declared pool in the config's
// declared order, then a single ad-hoc row (if any).
func buildRows(cfg *config.Config, specs []PaneSpec) ([]rowGroup, map[string]*Pane) {
	panes := make(map[string]*Pane, len(specs))
	for _, s := range specs {
		panes[s.Name] = NewPane(s)
	}

	// Pool rows preserve cfg.Workers.Queue declaration order, not
	// whatever hash order specs might otherwise fall in.
	poolOrder := make([]string, 0, len(cfg.Workers.Queue))
	poolIdx := make(map[string]int, len(cfg.Workers.Queue))
	for i, p := range cfg.Workers.Queue {
		poolOrder = append(poolOrder, p.Name)
		poolIdx[p.Name] = i
	}

	var scheduleIDs, adhocIDs []string
	poolIDs := make([][]string, len(poolOrder))

	for _, s := range specs {
		switch s.Kind {
		case KindSchedule:
			scheduleIDs = append(scheduleIDs, s.Name)
		case KindQueue:
			if i, ok := poolIdx[s.Pool]; ok {
				poolIDs[i] = append(poolIDs[i], s.Name)
			}
		case KindAdhoc:
			adhocIDs = append(adhocIDs, s.Name)
		}
	}

	var rows []rowGroup
	if len(scheduleIDs) > 0 {
		rows = append(rows, rowGroup{
			kind:    KindSchedule,
			label:   "schedule",
			paneIDs: scheduleIDs,
		})
	}
	for i, name := range poolOrder {
		if len(poolIDs[i]) == 0 {
			continue
		}
		rows = append(rows, rowGroup{
			kind:    KindQueue,
			label:   "pool:" + name,
			pool:    name,
			paneIDs: poolIDs[i],
		})
	}
	if len(adhocIDs) > 0 {
		rows = append(rows, rowGroup{
			kind:    KindAdhoc,
			label:   "adhoc",
			paneIDs: adhocIDs,
		})
	}
	return rows, panes
}

// Init returns the initial batch of subscriptions: one for the stats hub,
// one per log reader, and (if live) one for the reconciler.
// tea.WindowSizeMsg arrives automatically from the runtime.
func (m *TopModel) Init() tea.Cmd {
	cmds := []tea.Cmd{waitForStats(m.statsHub)}
	for id, r := range m.logsReaders {
		cmds = append(cmds, waitForLogsFor(id, r))
	}
	if m.reconciler != nil {
		cmds = append(cmds, waitForReconcile(m.reconciler))
	}
	return tea.Batch(cmds...)
}

// waitForStats returns a tea.Cmd that blocks on one stats snapshot from
// the hub. On receipt it returns a StatsMsg; the Update handler must
// re-subscribe by returning waitForStats again so we keep draining.
// A nil-map receive means the hub closed — return nil to unsubscribe.
func waitForStats(h *Hub) tea.Cmd {
	return func() tea.Msg {
		snap, ok := <-h.Updates()
		if !ok {
			return nil
		}
		return StatsMsg(snap)
	}
}

// waitForLogsFor is the paneID-aware log subscription used by Init and
// Update. We wrap the paneID into the closure so the EOF StateMsg knows
// which pane to transition without having to reach into LogsReader's
// internals.
func waitForLogsFor(paneID string, r *LogsReader) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-r.Lines()
		if !ok {
			return StateMsg{PaneID: paneID, State: StateExited}
		}
		return LogLineMsg{PaneID: line.PaneID, Line: line.Line}
	}
}

// waitForReconcile subscribes to the reconciler's event channel. On
// channel close it unsubscribes (nil return); otherwise it wraps the
// event in a reconcileMsg.
func waitForReconcile(r *Reconciler) tea.Cmd {
	return func() tea.Msg {
		evt, ok := <-r.Events()
		if !ok {
			return nil
		}
		return reconcileMsg{evt: evt}
	}
}

// Update is the main message pump. See the switch below for which
// messages are handled and which re-subscribe.
func (m *TopModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recomputeLayout()
		return m, m.resizeAllCmd()

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case StatsMsg:
		// Fan out to every pane, then re-subscribe.
		var cmds []tea.Cmd
		for _, p := range m.panesByID {
			_, c := p.Update(msg)
			if c != nil {
				cmds = append(cmds, c)
			}
		}
		cmds = append(cmds, waitForStats(m.statsHub))
		return m, tea.Batch(cmds...)

	case LogLineMsg:
		if p, ok := m.panesByID[msg.PaneID]; ok {
			p.Update(msg)
		}
		// Re-subscribe to this pane's reader (if still present).
		var cmd tea.Cmd
		if r, ok := m.logsReaders[msg.PaneID]; ok {
			cmd = waitForLogsFor(msg.PaneID, r)
		}
		return m, cmd

	case StateMsg:
		if p, ok := m.panesByID[msg.PaneID]; ok {
			p.Update(msg)
		}
		// EOF → the reader's goroutine is done. Drop it from the map
		// so we don't keep a dead subscription alive. No re-subscribe.
		delete(m.logsReaders, msg.PaneID)
		return m, nil

	case reconcileMsg:
		return m, m.handleReconcile(msg.evt)

	case paneCleanupMsg:
		m.removePane(msg.paneID)
		m.recomputeLayout()
		return m, m.resizeAllCmd()
	}
	return m, nil
}

// handleKey routes keys per the spec's Controls table.
func (m *TopModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "tab", "right", "l":
		m.focusShift(+1)
		return m, m.broadcastFocus()

	case "shift+tab", "left", "h":
		m.focusShift(-1)
		return m, m.broadcastFocus()

	case "enter":
		if m.focusedID != "" {
			m.zoomedID = m.focusedID
			m.recomputeLayout()
			return m, m.resizeAllCmd()
		}
		return m, nil

	case "esc":
		if m.zoomedID != "" {
			m.zoomedID = ""
			m.recomputeLayout()
			return m, m.resizeAllCmd()
		}
		return m, nil

	case "pgup", "pgdown", "g", "G":
		// Scrollback is zoom-only per the Controls table. In grid mode
		// these keys are no-ops; when zoomed we delegate to the focused
		// pane's viewport via its Update method.
		if m.zoomedID != "" {
			if p, ok := m.panesByID[m.zoomedID]; ok {
				p.viewport, _ = p.viewport.Update(msg)
			}
		}
		return m, nil
	}
	return m, nil
}

// handleReconcile mutates state for one reconciler event and returns the
// commands needed to integrate it (new subscription for Adds, cleanup
// timer for Removes). Always re-subscribes to the reconciler channel.
func (m *TopModel) handleReconcile(evt ReconcileEvent) tea.Cmd {
	var cmds []tea.Cmd

	switch evt.Type {
	case EventAdd:
		// Guard: reconciler already dedup's against its own snapshot, but
		// a pane could still exist from cold-start discovery.
		if _, exists := m.panesByID[evt.Spec.Name]; !exists {
			p := NewPane(evt.Spec)
			m.panesByID[evt.Spec.Name] = p
			m.appendAdhocRow(evt.Spec.Name)

			r := NewLogsReader(evt.Spec, m.opts.logsExec)
			m.logsReaders[evt.Spec.Name] = r
			// Run under a background context — the TopModel doesn't own
			// svcCtx at this depth. Readers are reaped via their own
			// ctx.Cancel through tea.WithContext's chain: the outer Run
			// cancels svcCtx, which bubbletea uses for exec.CommandContext
			// in DefaultCmdStartFn... but NewLogsReader already captured
			// its exec fn with whatever ctx Init received. For ad-hoc
			// additions we spawn under context.Background — shutdown comes
			// via the per-reader EOF path when the container disappears.
			go r.Run(context.Background())
			cmds = append(cmds, waitForLogsFor(evt.Spec.Name, r))

			m.recomputeLayout()
			cmds = append(cmds, m.resizeAllCmd())
		}

	case EventRemove:
		// Flip the pane to exited so the border turns red; keep it on
		// screen for a 10s grace window per the spec, then fire a
		// paneCleanupMsg to garbage-collect.
		if p, ok := m.panesByID[evt.Spec.Name]; ok {
			p.Update(StateMsg{PaneID: evt.Spec.Name, State: StateExited})
		}
		cmds = append(cmds, cleanupAfter(evt.Spec.Name, 10*time.Second))
	}

	cmds = append(cmds, waitForReconcile(m.reconciler))
	return tea.Batch(cmds...)
}

// cleanupAfter schedules a paneCleanupMsg after d.
func cleanupAfter(paneID string, d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return paneCleanupMsg{paneID: paneID}
	})
}

// removePane tears down a pane and any still-live reader, and strips it
// from whichever row contains it.
func (m *TopModel) removePane(paneID string) {
	if r, ok := m.logsReaders[paneID]; ok {
		// Reader may still be alive if EventRemove fired before EOF —
		// closing stdout via wait() requires ctx cancellation, which we
		// don't control here. The reader will reap when the container's
		// docker logs subprocess hits EOF or when the outer ctx fires.
		_ = r
		delete(m.logsReaders, paneID)
	}
	delete(m.panesByID, paneID)

	for i := range m.rowOrder {
		row := &m.rowOrder[i]
		for j, id := range row.paneIDs {
			if id == paneID {
				row.paneIDs = append(row.paneIDs[:j], row.paneIDs[j+1:]...)
				break
			}
		}
	}
	// Drop now-empty rows so the layout doesn't render hollow space.
	pruned := m.rowOrder[:0]
	for _, row := range m.rowOrder {
		if len(row.paneIDs) > 0 {
			pruned = append(pruned, row)
		}
	}
	m.rowOrder = pruned

	if m.focusedID == paneID {
		m.focusedID = m.firstPaneID()
	}
	if m.zoomedID == paneID {
		m.zoomedID = ""
	}
}

// appendAdhocRow adds a paneID to the trailing ad-hoc row, creating the
// row if none exists yet.
func (m *TopModel) appendAdhocRow(paneID string) {
	for i := range m.rowOrder {
		if m.rowOrder[i].kind == KindAdhoc {
			m.rowOrder[i].paneIDs = append(m.rowOrder[i].paneIDs, paneID)
			return
		}
	}
	m.rowOrder = append(m.rowOrder, rowGroup{
		kind:    KindAdhoc,
		label:   "adhoc",
		paneIDs: []string{paneID},
	})
}

// focusShift moves focus +/- one pane across the flattened pane order.
// Empty fleets are a no-op.
func (m *TopModel) focusShift(delta int) {
	order := m.flatPaneOrder()
	if len(order) == 0 {
		return
	}
	idx := 0
	for i, id := range order {
		if id == m.focusedID {
			idx = i
			break
		}
	}
	idx = (idx + delta + len(order)) % len(order)
	m.focusedID = order[idx]
}

// flatPaneOrder is the focus-cycle order: row-major, left-to-right.
func (m *TopModel) flatPaneOrder() []string {
	var out []string
	for _, row := range m.rowOrder {
		out = append(out, row.paneIDs...)
	}
	return out
}

// firstPaneID returns the first pane in focus order, or "" for an empty
// grid.
func (m *TopModel) firstPaneID() string {
	for _, row := range m.rowOrder {
		if len(row.paneIDs) > 0 {
			return row.paneIDs[0]
		}
	}
	return ""
}

// broadcastFocus sends a FocusMsg to every pane so borders update.
func (m *TopModel) broadcastFocus() tea.Cmd {
	for id, p := range m.panesByID {
		p.Update(FocusMsg{PaneID: id, Focused: id == m.focusedID})
	}
	return nil
}

// recomputeLayout resolves the current grid dimensions from m.width/m.height
// and m.rowOrder. Called on WindowSizeMsg, zoom toggles, and pane add/remove.
func (m *TopModel) recomputeLayout() {
	specs := make([]RowSpec, len(m.rowOrder))
	for i, r := range m.rowOrder {
		specs[i] = RowSpec{Label: r.label, PaneCount: len(r.paneIDs)}
	}
	m.layout = ComputeLayout(m.width, m.height, specs, m.opts.MinPaneWidth)
	m.recomputeBounds()
}

// recomputeBounds rebuilds paneBounds from the current layout + zoom state.
// In zoom mode every rect is the full body area (so any click unzooms).
func (m *TopModel) recomputeBounds() {
	m.paneBounds = make(map[string]paneRect)
	if m.zoomedID != "" {
		h := m.height - m.layout.HeaderHeight - m.layout.FooterHeight
		if h < 1 {
			h = 1
		}
		m.paneBounds[m.zoomedID] = paneRect{
			x: 0, y: m.layout.HeaderHeight, w: m.width, h: h,
		}
		return
	}
	y := m.layout.HeaderHeight
	for i, row := range m.rowOrder {
		if i >= len(m.layout.Rows) {
			break
		}
		rl := m.layout.Rows[i]
		x := 0
		for j, paneID := range row.paneIDs {
			if j >= len(rl.Panes) {
				break
			}
			pl := rl.Panes[j]
			m.paneBounds[paneID] = paneRect{x: x, y: y, w: pl.Width, h: rl.Height}
			x += pl.Width
		}
		y += rl.Height
	}
}

// handleMouse implements click-to-zoom per the Controls extension.
// Left-click on a pane: if not zoomed → zoom into it; if zoomed into the
// same pane → unzoom; if zoomed into a different pane → switch zoom target.
// Other buttons and motion events are ignored.
func (m *TopModel) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	for paneID, r := range m.paneBounds {
		if !r.contains(msg.X, msg.Y) {
			continue
		}
		if m.zoomedID == paneID {
			// Click on zoomed pane → unzoom.
			m.zoomedID = ""
		} else {
			m.focusedID = paneID
			m.zoomedID = paneID
		}
		m.recomputeLayout()
		return m, tea.Batch(m.broadcastFocus(), m.resizeAllCmd())
	}
	return m, nil
}

// resizeAllCmd dispatches a ResizeMsg to every pane based on the current
// layout (or zoom state). Invoked whenever dimensions or membership change.
func (m *TopModel) resizeAllCmd() tea.Cmd {
	if m.zoomedID != "" {
		p, ok := m.panesByID[m.zoomedID]
		if !ok {
			return nil
		}
		// Zoom: pane fills the full body (everything except header + footer).
		h := m.height - m.layout.HeaderHeight - m.layout.FooterHeight
		if h < 1 {
			h = 1
		}
		p.Update(ResizeMsg{
			PaneID:        m.zoomedID,
			Width:         m.width,
			Height:        h,
			TruncateTitle: false,
		})
		return nil
	}

	// Grid mode: walk rows in lockstep with layout.
	for i, row := range m.rowOrder {
		if i >= len(m.layout.Rows) {
			break
		}
		rl := m.layout.Rows[i]
		for j, paneID := range row.paneIDs {
			if j >= len(rl.Panes) {
				break
			}
			pl := rl.Panes[j]
			if p, ok := m.panesByID[paneID]; ok {
				p.Update(ResizeMsg{
					PaneID:        paneID,
					Width:         pl.Width,
					Height:        pl.Height,
					TruncateTitle: rl.TruncateTitles,
				})
			}
		}
	}
	return nil
}

// View renders the header, grid (or zoom), and footer.
func (m *TopModel) View() string {
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteByte('\n')

	if m.zoomedID != "" {
		if p, ok := m.panesByID[m.zoomedID]; ok {
			b.WriteString(p.View())
			b.WriteByte('\n')
		}
	} else {
		for i, row := range m.rowOrder {
			if i >= len(m.layout.Rows) {
				break
			}
			panes := make([]string, 0, len(row.paneIDs))
			for _, paneID := range row.paneIDs {
				if p, ok := m.panesByID[paneID]; ok {
					panes = append(panes, p.View())
				}
			}
			if len(panes) > 0 {
				b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, panes...))
				b.WriteByte('\n')
			}
		}
	}

	b.WriteString(m.footer())
	return b.String()
}

// header renders the top status line.
func (m *TopModel) header() string {
	mode := "snapshot"
	if m.opts.Live {
		mode = "live"
	}
	focus := m.focusedID
	if focus == "" {
		focus = "no focus"
	}
	text := fmt.Sprintf("frank worker top · %s · %s · %s", m.projectName, mode, focus)
	return Header.Render(text)
}

// footer renders the bottom key-hint line, swapping hints when zoomed.
func (m *TopModel) footer() string {
	var text string
	if m.zoomedID != "" {
		text = "esc / click back · pgup/pgdn scroll · q quit"
	} else {
		text = "q quit · tab focus · enter / click zoom · esc back"
	}
	return Footer.Render(text)
}

// waitTimeout waits on wg for up to d. If wg doesn't complete, it returns
// anyway — the caller is shutting down and can't afford to block forever.
func waitTimeout(wg *sync.WaitGroup, d time.Duration) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(d):
	}
}

// --- docker adapters -------------------------------------------------

// dockerInspector adapts *docker.Client to the containerInspector
// interface that discoverWorkers expects.
type dockerInspector struct {
	c *docker.Client
}

func (d *dockerInspector) InspectContainer(name string) (string, int, string, error) {
	return d.c.InspectContainer(name)
}

func (d *dockerInspector) AdhocWorkerNames(projectName string) ([]string, error) {
	return d.c.AdhocWorkerNames(projectName)
}

// dockerLister adapts *docker.Client to the adhocLister interface that
// the reconciler expects. Bridges the docker.AdhocWorker concrete type
// over to the package-local AdhocContainer.
type dockerLister struct {
	c           *docker.Client
	projectName string
}

func (d *dockerLister) ListAdhocWorkers() ([]AdhocContainer, error) {
	workers, err := d.c.ListAdhocWorkersWithIDs(d.projectName)
	if err != nil {
		return nil, err
	}
	out := make([]AdhocContainer, len(workers))
	for i, w := range workers {
		out[i] = AdhocContainer{ID: w.ID, Name: w.Name}
	}
	return out, nil
}
