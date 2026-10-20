package prssection

import (
	"fmt"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/data"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prrow"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/theme"
)

func rowsTestSection(t *testing.T, n int) *Model {
	t.Helper()
	cfg, err := config.ParseConfig(config.Location{
		ConfigFlag:       "../../../config/testdata/test-config.yml",
		SkipGlobalConfig: true,
	})
	require.NoError(t, err)
	ctx := &context.ProgramContext{
		Config: &cfg, ScreenWidth: 160, ScreenHeight: 40, MainContentWidth: 160, MainContentHeight: 36,
		View: config.PRsView, StartTask: func(task context.Task) tea.Cmd { return nil },
	}
	ctx.Theme = theme.ParseTheme(ctx.Config)
	ctx.Styles = context.InitStyles(ctx.Theme)

	m := NewModel(1, ctx, config.PrsSectionConfig{Title: "Mine", Filters: "is:open"}, time.Now(), time.Now())
	for i := range n {
		num := 500 + i
		m.Prs = append(m.Prs, prrow.Data{Primary: &data.PullRequestData{
			Number: num, Title: fmt.Sprintf("title %d", num), State: "OPEN",
			Url:        fmt.Sprintf("https://github.com/acme/monorepo/pull/%d", num),
			Repository: data.Repository{NameWithOwner: "acme/monorepo"},
			// Two days old: ages are formatted to the second, so "now" would turn
			// into "1s" on a slow run and fail the comparison for the wrong reason.
			UpdatedAt: time.Now().Add(-48 * time.Hour), CreatedAt: time.Now().Add(-48 * time.Hour),
		}})
	}
	m.Table.SetRows(m.BuildRows())

	return &m
}

// The selected row's cells carry selected styling (title, labels), so a cursor
// move must restyle the rows it left and entered. However the section gets
// there, the rows on screen must always be exactly what a full rebuild produces.
func TestRows_AlwaysMatchAFullRebuildAcrossCursorMoves(t *testing.T) {
	m := rowsTestSection(t, 25)
	tick := spinner.TickMsg{}

	check := func(what string) {
		t.Helper()
		require.Equal(t, m.BuildRows(), m.Table.Rows, what)
	}

	for range 4 {
		m.NextRow()
		_, _ = m.Update(tick)
	}
	check("after moving down")

	m.PrevRow()
	m.PrevRow()
	_, _ = m.Update(tick)
	check("after moving back up")

	m.SetCurrRow(20)
	_, _ = m.Update(tick)
	check("after a jump")

	m.LastItem()
	m.FirstItem()
	_, _ = m.Update(tick)
	check("after moving twice between messages")
}

// Row cells are formatted for their column widths (the title is truncated when
// the cells are built), so a width change -- a resize, toggling the preview,
// dragging the divider -- must rebuild them, not just re-render the old ones.
// Every tab, not only the one on screen: the next switch must not show titles
// cut for a width the window no longer has.
func TestRows_FollowAWidthChange(t *testing.T) {
	m := rowsTestSection(t, 10)
	for i := range m.Prs {
		m.Prs[i].Primary.Title = "a title long enough that it is truncated in a narrower list " + m.Prs[i].Primary.Title
	}
	m.Table.SetRows(m.BuildRows())

	for _, w := range []int{100, 160, 70} {
		ctx := *m.Ctx
		ctx.MainContentWidth = w
		m.UpdateProgramContext(&ctx) // what the program does to every tab after a layout change
		require.Equal(t, m.BuildRows(), m.Table.Rows, "after the list became %d wide", w)
	}
}
