package prssection

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/data"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prrow"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/section"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/theme"
)

func refreshTestCtx(t *testing.T) *context.ProgramContext {
	t.Helper()
	cfg, err := config.ParseConfig(config.Location{
		ConfigFlag:       "../../../config/testdata/test-config.yml",
		SkipGlobalConfig: true,
	})
	require.NoError(t, err)
	ctx := &context.ProgramContext{
		Config:            &cfg,
		ScreenWidth:       100,
		ScreenHeight:      40,
		MainContentWidth:  100,
		MainContentHeight: 20,
		View:              config.PRsView,
		StartTask:         func(task context.Task) tea.Cmd { return nil },
	}
	ctx.Theme = theme.ParseTheme(ctx.Config)
	ctx.Styles = context.InitStyles(ctx.Theme)
	return ctx
}

func prAt(n int) prrow.Data {
	return prrow.Data{Primary: &data.PullRequestData{
		Number: n,
		Url:    "https://github.com/o/r/pull/" + string(rune('0'+n)),
	}}
}

// Regression: a refresh must NOT flash the cursor to the top of the list. The
// refresh rebuilds each section from scratch (fresh table, cursor 0) carrying
// the old rows; if the selection is only restored once the fetch lands, the
// list renders at the top in between -- jarring, and a keypress in that window
// acts on the wrong PR. FetchAllSections must carry the cursor over immediately.
func TestRefresh_SelectionDoesNotFlashToTop(t *testing.T) {
	ctx := refreshTestCtx(t)

	// An existing section with 4 PRs, the 3rd selected.
	old := NewModel(1, ctx, ctx.Config.PRSections[0], time.Now(), time.Now())
	old.Prs = []prrow.Data{prAt(1), prAt(2), prAt(3), prAt(4)}
	old.Table.SetRows(old.BuildRows())
	old.Table.SetCurrItem(2)
	wantURL := old.GetCurrRow().GetUrl()
	require.Equal(t, "https://github.com/o/r/pull/3", wantURL, "precondition: 3rd PR selected")

	// Refresh: index 0 is the search slot, real sections start at 1.
	oldPrs := make([]section.Section, len(ctx.Config.PRSections)+1)
	oldPrs[1] = &old
	newSections, _ := FetchAllSections(ctx, oldPrs)

	ns := newSections[0].(*Model)
	// The render loop rebuilds rows every Update, BEFORE the fetch lands.
	ns.Table.SetRows(ns.BuildRows())

	require.Equal(t, 2, ns.Table.GetCurrItem(),
		"cursor must carry over on refresh, not flash to the top row")
	require.NotNil(t, ns.GetCurrRow())
	require.Equal(t, wantURL, ns.GetCurrRow().GetUrl(),
		"the same PR must stay selected across the refresh")
}
