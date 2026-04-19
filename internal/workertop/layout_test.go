package workertop

import "testing"

func TestComputeLayout_ZeroRows(t *testing.T) {
	got := ComputeLayout(80, 24, nil, 30)
	if got.HeaderHeight != 1 || got.FooterHeight != 1 {
		t.Fatalf("expected header/footer = 1/1, got %d/%d", got.HeaderHeight, got.FooterHeight)
	}
	if len(got.Rows) != 0 {
		t.Fatalf("expected zero rows, got %d", len(got.Rows))
	}
	if got.Vertical {
		t.Fatalf("expected Vertical=false for empty layout")
	}
}

func TestComputeLayout_NarrowStandard80x24(t *testing.T) {
	// 80x24, 4 rows of 1 pane each.
	// budget = 24 - 2 = 22; perRow = 22/4 = 5 (at minimum, not vertical).
	rows := []RowSpec{
		{Label: "schedule", PaneCount: 1},
		{Label: "pool:default", PaneCount: 1},
		{Label: "pool:billing", PaneCount: 1},
		{Label: "adhoc", PaneCount: 1},
	}
	got := ComputeLayout(80, 24, rows, 30)

	if got.Vertical {
		t.Fatalf("22/4 = 5 meets the minimum; expected Vertical=false")
	}
	if len(got.Rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(got.Rows))
	}
	for i, r := range got.Rows {
		if r.Height != 5 {
			t.Errorf("row %d: height %d, want 5", i, r.Height)
		}
		if len(r.Panes) != 1 {
			t.Errorf("row %d: pane count %d, want 1", i, len(r.Panes))
		}
		if r.Panes[0].Width != 80 {
			t.Errorf("row %d: pane width %d, want 80", i, r.Panes[0].Width)
		}
		if r.Paginated {
			t.Errorf("row %d: unexpected Paginated=true", i)
		}
		if r.TruncateTitles {
			t.Errorf("row %d: unexpected TruncateTitles=true (80 >= 30)", i)
		}
	}
}

func TestComputeLayout_Ultrawide320x80(t *testing.T) {
	// schedule=1, pool=3, pool=2, adhoc=2 → 4 rows.
	// budget = 78; perRow = 78/4 = 19. Generous heights.
	rows := []RowSpec{
		{Label: "schedule", PaneCount: 1},
		{Label: "pool:default", PaneCount: 3},
		{Label: "pool:billing", PaneCount: 2},
		{Label: "adhoc", PaneCount: 2},
	}
	got := ComputeLayout(320, 80, rows, 30)

	if got.Vertical {
		t.Fatal("ultrawide should not force vertical scroll")
	}
	if len(got.Rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(got.Rows))
	}
	wantWidths := []int{320, 320 / 3, 320 / 2, 320 / 2}
	wantCounts := []int{1, 3, 2, 2}
	for i, r := range got.Rows {
		if r.Height != 19 {
			t.Errorf("row %d: height %d, want 19", i, r.Height)
		}
		if len(r.Panes) != wantCounts[i] {
			t.Errorf("row %d: pane count %d, want %d", i, len(r.Panes), wantCounts[i])
		}
		if r.Panes[0].Width != wantWidths[i] {
			t.Errorf("row %d: pane width %d, want %d", i, r.Panes[0].Width, wantWidths[i])
		}
		if r.Paginated {
			t.Errorf("row %d: unexpected Paginated=true", i)
		}
		if r.TruncateTitles {
			t.Errorf("row %d: unexpected TruncateTitles=true", i)
		}
	}
}

func TestComputeLayout_PaginatedSingleRow9Panes80Cols(t *testing.T) {
	// 9 panes at 80 cols → colWidth 8 → paginate.
	// panesPerPage = 80/20 = 4; pageCount = ceil(9/4) = 3; pageWidth = 80/4 = 20.
	rows := []RowSpec{{Label: "pool:default", PaneCount: 9}}
	got := ComputeLayout(80, 24, rows, 30)

	if len(got.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(got.Rows))
	}
	r := got.Rows[0]
	if !r.Paginated {
		t.Fatal("expected Paginated=true")
	}
	if r.PanesPerPage != 4 {
		t.Errorf("PanesPerPage = %d, want 4", r.PanesPerPage)
	}
	if r.PageCount != 3 {
		t.Errorf("PageCount = %d, want 3", r.PageCount)
	}
	if len(r.Panes) != 4 {
		t.Errorf("rendered pane slots = %d, want 4 (panes per page)", len(r.Panes))
	}
	for _, p := range r.Panes {
		if p.Width != 20 {
			t.Errorf("pane width %d, want 20", p.Width)
		}
	}
	if !r.TruncateTitles {
		t.Error("expected TruncateTitles=true (20 < 30)")
	}
}

func TestComputeLayout_VeryShortTerminal(t *testing.T) {
	// 80x10, 3 rows. budget = 8, perRow = 8/3 = 2 < 5 → Vertical.
	rows := []RowSpec{
		{Label: "schedule", PaneCount: 1},
		{Label: "pool:default", PaneCount: 1},
		{Label: "adhoc", PaneCount: 1},
	}
	got := ComputeLayout(80, 10, rows, 30)

	if !got.Vertical {
		t.Fatal("expected Vertical=true when budget < rows*minRowHeight")
	}
	for i, r := range got.Rows {
		if r.Height != 5 {
			t.Errorf("row %d: height %d, want 5 (virtual min)", i, r.Height)
		}
	}
}

func TestComputeLayout_MinPaneWidthTruncatesWithoutPaginating(t *testing.T) {
	// w=100, 3 panes, minPaneWidth=40: colWidth = 33 < 40 but > 20.
	// Expect TruncateTitles=true, Paginated=false.
	rows := []RowSpec{{Label: "pool:default", PaneCount: 3}}
	got := ComputeLayout(100, 24, rows, 40)

	r := got.Rows[0]
	if r.Paginated {
		t.Error("expected Paginated=false (33 >= 20)")
	}
	if !r.TruncateTitles {
		t.Error("expected TruncateTitles=true (33 < 40)")
	}
	if len(r.Panes) != 3 {
		t.Fatalf("expected 3 panes, got %d", len(r.Panes))
	}
	if r.Panes[0].Width != 33 {
		t.Errorf("pane width %d, want 33", r.Panes[0].Width)
	}
}

func TestComputeLayout_BoundaryExactly20Cols(t *testing.T) {
	// 4 panes at 80 cols → colWidth = 20 exactly. Not paginated (only
	// strictly less than 20 triggers pagination). TruncateTitles=true
	// against the default minPaneWidth=30.
	rows := []RowSpec{{Label: "pool", PaneCount: 4}}
	got := ComputeLayout(80, 24, rows, 30)

	r := got.Rows[0]
	if r.Paginated {
		t.Errorf("colWidth == 20 must not paginate (threshold is strict)")
	}
	if !r.TruncateTitles {
		t.Errorf("colWidth 20 < minPaneWidth 30 should truncate titles")
	}
	if len(r.Panes) != 4 {
		t.Fatalf("expected 4 panes, got %d", len(r.Panes))
	}
	if r.Panes[0].Width != 20 {
		t.Errorf("pane width %d, want 20", r.Panes[0].Width)
	}
}

func TestComputeLayout_HeaderFooterAlwaysOne(t *testing.T) {
	cases := []struct {
		name string
		w, h int
		rows []RowSpec
	}{
		{"zero everything", 0, 0, nil},
		{"tiny with rows", 10, 3, []RowSpec{{Label: "x", PaneCount: 1}}},
		{"huge", 400, 200, []RowSpec{{Label: "x", PaneCount: 2}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeLayout(tc.w, tc.h, tc.rows, 30)
			if got.HeaderHeight != 1 || got.FooterHeight != 1 {
				t.Errorf("header/footer = %d/%d, want 1/1", got.HeaderHeight, got.FooterHeight)
			}
		})
	}
}

func TestComputeLayout_ZeroPaneCountRowTreatedAsOne(t *testing.T) {
	// Defensive: a row with PaneCount == 0 should not crash layout; it
	// gets treated as a single full-width pane so rendering can proceed.
	rows := []RowSpec{{Label: "empty", PaneCount: 0}}
	got := ComputeLayout(80, 24, rows, 30)
	if len(got.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(got.Rows))
	}
	r := got.Rows[0]
	if len(r.Panes) != 1 {
		t.Fatalf("expected 1 pane, got %d", len(r.Panes))
	}
	if r.Panes[0].Width != 80 {
		t.Errorf("pane width %d, want 80", r.Panes[0].Width)
	}
}

func TestComputeLayout_DefaultsWhenMinPaneWidthZero(t *testing.T) {
	// minPaneWidth <= 0 should fall back to the default (30).
	rows := []RowSpec{{Label: "pool", PaneCount: 3}}
	// 100/3 = 33, which is >= 30. Expect TruncateTitles=false under default.
	got := ComputeLayout(100, 24, rows, 0)
	if got.Rows[0].TruncateTitles {
		t.Errorf("default minPaneWidth=30 should not truncate colWidth=33")
	}
}

// TestComputeLayout_WideRowWithManyPanes confirms that when the row is
// wide enough that each pane comfortably exceeds minPaneWidth, neither
// pagination nor title truncation kicks in.
func TestComputeLayout_WideRowWithManyPanes(t *testing.T) {
	rows := []RowSpec{{Label: "pool", PaneCount: 4}}
	got := ComputeLayout(200, 40, rows, 30)
	r := got.Rows[0]
	if r.Paginated {
		t.Error("200/4 = 50 should not paginate")
	}
	if r.TruncateTitles {
		t.Error("50 >= 30 should not truncate")
	}
	if len(r.Panes) != 4 {
		t.Fatalf("expected 4 panes, got %d", len(r.Panes))
	}
	if r.Panes[0].Width != 50 {
		t.Errorf("pane width %d, want 50", r.Panes[0].Width)
	}
}
