// Package workertop provides the layout and runtime for `frank worker top`.
//
// This file contains the pure layout math: given terminal dimensions and a
// set of row specs, compute how many lines each row gets, how wide each
// pane is, and whether the row needs horizontal pagination or title
// truncation. No I/O, no dependencies outside the standard library (and in
// fact this file uses none).
package workertop

// RowSpec describes a logical row the TUI wants to render. Each row is one
// of: the scheduler pane, one declared queue pool (containing N queue:work
// replicas), or the ad-hoc row (containing every ad-hoc container
// discovered at cold start).
type RowSpec struct {
	// Label is a debugging tag — e.g. "schedule", "pool:default", "adhoc".
	// Not rendered; used by tests and logs.
	Label string
	// PaneCount is the number of panes to fit in this row. Must be >= 1
	// for the row to contribute any layout; a row with PaneCount == 0 is
	// laid out as a single empty pane so callers don't crash on division.
	PaneCount int
}

// Layout is the result of ComputeLayout: a fully-resolved plan the renderer
// can walk without further decisions.
type Layout struct {
	HeaderHeight int
	FooterHeight int
	Rows         []RowLayout
	// Vertical is true when the budget (h - 2) cannot fit every row at
	// the minimum 5-line height. The caller is responsible for wrapping
	// the rows in a vertical scroll viewport. Each RowLayout.Height is
	// still clamped to the 5-line minimum so the renderer can draw the
	// full virtual height.
	Vertical bool
}

// RowLayout is a single row after layout resolution.
type RowLayout struct {
	Label  string
	Height int
	Panes  []PaneLayout
	// Paginated is true when the terminal is too narrow to fit all panes
	// side-by-side at >= 20 cols each. The renderer should draw only the
	// current page (PanesPerPage panes at a time) and show a `◂ p/N ▸`
	// indicator.
	Paginated    bool
	PageCount    int
	PanesPerPage int
	// TruncateTitles is true when each pane's column width fell below the
	// caller-supplied minPaneWidth threshold. The pane renderer should
	// truncate titles (e.g. `laravel.queue.default.1` → `laravel.q.def.1`)
	// to fit. Independent of Paginated: a row can have narrow-but-
	// readable panes (20 <= colWidth < minPaneWidth) where truncation
	// applies but pagination does not.
	TruncateTitles bool
}

// PaneLayout is the final size of a single pane cell.
type PaneLayout struct {
	Width  int
	Height int
}

// Hard-coded structural thresholds. Pagination kicks in below 20 cols
// because pane titles and log lines are unreadable narrower than that.
// minPaneWidth (the caller-supplied soft hint) only drives title
// truncation.
const (
	paginationThreshold = 20
	minRowHeight        = 5
	defaultMinPaneWidth = 30
)

// ComputeLayout is the single entry point. Given terminal dimensions
// (w, h), a list of row specs, and the minimum pane width at which full
// titles still fit, it returns a Layout describing every row and pane.
//
// The function is total: any w, h, or rows value (including zero or
// negative) produces a valid Layout. Degenerate inputs collapse to an
// empty Layout with HeaderHeight/FooterHeight still set to 1 so the
// caller can always draw chrome.
func ComputeLayout(w, h int, rows []RowSpec, minPaneWidth int) Layout {
	if minPaneWidth <= 0 {
		minPaneWidth = defaultMinPaneWidth
	}

	layout := Layout{
		HeaderHeight: 1,
		FooterHeight: 1,
	}

	if len(rows) == 0 {
		return layout
	}

	budget := h - layout.HeaderHeight - layout.FooterHeight
	if budget < 0 {
		budget = 0
	}

	visibleRows := len(rows)
	perRow := 0
	if visibleRows > 0 {
		perRow = budget / visibleRows
	}
	if perRow < minRowHeight {
		layout.Vertical = true
		perRow = minRowHeight
	}

	layout.Rows = make([]RowLayout, 0, visibleRows)
	for _, spec := range rows {
		layout.Rows = append(layout.Rows, computeRow(w, perRow, spec, minPaneWidth))
	}

	return layout
}

// computeRow resolves a single row's column math. Width w is the full
// terminal width; perRow is the already-resolved row height.
func computeRow(w, perRow int, spec RowSpec, minPaneWidth int) RowLayout {
	paneCount := spec.PaneCount
	if paneCount < 1 {
		paneCount = 1
	}

	row := RowLayout{
		Label:  spec.Label,
		Height: perRow,
	}

	// Clamp width to a non-negative value; a zero-width terminal still
	// produces a structurally valid row with zero-width panes.
	if w < 0 {
		w = 0
	}

	colWidth := 0
	if paneCount > 0 {
		colWidth = w / paneCount
	}

	if colWidth < paginationThreshold {
		// Horizontal pagination: fit as many 20-col panes as we can,
		// minimum one, and spread them to evenly divide the width.
		panesPerPage := w / paginationThreshold
		if panesPerPage < 1 {
			panesPerPage = 1
		}
		if panesPerPage > paneCount {
			panesPerPage = paneCount
		}

		pageCount := (paneCount + panesPerPage - 1) / panesPerPage

		pageWidth := 0
		if panesPerPage > 0 {
			pageWidth = w / panesPerPage
		}

		row.Paginated = true
		row.PanesPerPage = panesPerPage
		row.PageCount = pageCount
		row.TruncateTitles = pageWidth < minPaneWidth

		row.Panes = make([]PaneLayout, panesPerPage)
		for i := range row.Panes {
			row.Panes[i] = PaneLayout{Width: pageWidth, Height: perRow}
		}
		return row
	}

	// Non-paginated: every pane shares the row width evenly.
	row.PageCount = 1
	row.PanesPerPage = paneCount
	row.TruncateTitles = colWidth < minPaneWidth

	row.Panes = make([]PaneLayout, paneCount)
	for i := range row.Panes {
		row.Panes[i] = PaneLayout{Width: colWidth, Height: perRow}
	}
	return row
}
