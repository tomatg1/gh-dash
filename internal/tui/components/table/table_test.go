package table

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/constants"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/theme"
)

func testCtx(t *testing.T) *context.ProgramContext {
	t.Helper()
	cfg, err := config.ParseConfig(config.Location{
		ConfigFlag:       "../../../config/testdata/test-config.yml",
		SkipGlobalConfig: true,
	})
	require.NoError(t, err)
	ctx := &context.ProgramContext{Config: &cfg}
	ctx.Theme = theme.ParseTheme(ctx.Config)
	ctx.Styles = context.InitStyles(ctx.Theme)

	return ctx
}

func intp(n int) *int    { return &n }
func boolp(b bool) *bool { return &b }

func testColumns() []Column {
	return []Column{
		{Title: "#", Width: intp(7)},
		{Title: "Title", Grow: boolp(true)},
		{Title: "Age", Width: intp(5)},
	}
}

func testRows(n int, tag string) []Row {
	rows := make([]Row, n)
	for i := range n {
		rows[i] = Row{fmt.Sprintf("#%d", 100+i), fmt.Sprintf("%s title %d\nsecond line %d", tag, i, i), fmt.Sprintf("%dh", i)}
	}

	return rows
}

func newTable(ctx *context.ProgramContext, dims constants.Dimensions, cols []Column, rows []Row) Model {
	now := time.Now()
	m := NewModel(*ctx, dims, now, now, cols, nil, "item", nil, "", false)
	m.SetRows(rows)

	return m
}

// fromScratch renders the same rows, columns and scroll state with no history,
// which is what the incremental render has to match exactly.
func fromScratch(ctx *context.ProgramContext, m Model) string {
	cols := append([]Column(nil), m.Columns...)
	rows := append([]Row(nil), m.Rows...)
	fresh := newTable(ctx, m.dimensions, cols, rows)
	fresh.RestoreScroll(m.ScrollState())

	return fresh.View()
}

// Rendering is cached per row and only what changed is re-rendered. Whatever the
// cache skips, the screen must be byte-for-byte what a full render produces --
// faster must never mean stale.
func TestIncrementalRender_MatchesAFullRender(t *testing.T) {
	ctx := testCtx(t)
	dims := constants.Dimensions{Width: 70, Height: 15}
	m := newTable(ctx, dims, testColumns(), testRows(12, "a"))

	check := func(what string) {
		t.Helper()
		require.Equal(t, fromScratch(ctx, m), m.View(), what)
	}
	check("initial")

	for range 3 {
		m.NextItem()
	}
	check("after moving the cursor")

	rows := testRows(12, "a")
	rows[5][1] = "changed title\nsecond line"
	rows[3][2] = "9d"
	m.SetRows(rows)
	check("after changing two rows' cells, one of them selected")

	m.SetRows(testRows(12, "a")[:12])
	check("after reverting them")

	m.SetDimensions(constants.Dimensions{Width: 55, Height: 15})
	m.SyncViewPortContent() // as section.UpdateProgramContext does after resizing
	check("after a resize")

	for range 9 {
		m.NextItem()
	}
	check("after scrolling past a page")

	m.SetRows(testRows(6, "a"))
	check("after the list shrank under the cursor")

	m.SetRows(testRows(20, "b"))
	check("after the list grew with all-new content")

	m.PrevItem()
	m.SetCurrItem(14)
	check("after a jump")

	m.Columns[2].Hidden = boolp(true)
	m.SyncViewPortContent()
	check("after hiding a column")
}
