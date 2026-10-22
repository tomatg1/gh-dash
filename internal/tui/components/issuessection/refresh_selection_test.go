package issuessection

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/data"
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
		View:              config.IssuesView,
		StartTask:         func(task context.Task) tea.Cmd { return nil },
	}
	ctx.Theme = theme.ParseTheme(ctx.Config)
	ctx.Styles = context.InitStyles(ctx.Theme)
	return ctx
}

// Regression: a refresh must not blank the issue list or flash the cursor to the
// top before the fetch lands. FetchAllSections must carry the rows AND cursor
// over immediately.
func TestRefresh_IssueSelectionDoesNotFlashToTop(t *testing.T) {
	ctx := refreshTestCtx(t)

	old := NewModel(1, ctx, ctx.Config.IssuesSections[0], time.Now(), time.Now())
	old.Issues = []data.IssueData{
		{Number: 1, Url: "https://github.com/o/r/issues/1"},
		{Number: 2, Url: "https://github.com/o/r/issues/2"},
		{Number: 3, Url: "https://github.com/o/r/issues/3"},
		{Number: 4, Url: "https://github.com/o/r/issues/4"},
	}
	old.Table.SetRows(old.BuildRows())
	old.Table.SetCurrItem(2)
	wantURL := old.GetCurrRow().GetUrl()
	require.Equal(t, "https://github.com/o/r/issues/3", wantURL, "precondition: 3rd issue selected")

	oldIssues := make([]section.Section, len(ctx.Config.IssuesSections)+1)
	oldIssues[1] = &old
	newSections, _ := FetchAllSections(ctx, oldIssues)

	ns := newSections[0].(*Model)
	ns.Table.SetRows(ns.BuildRows()) // the render loop rebuilds rows before the fetch lands

	require.Equal(t, 4, len(ns.Issues), "carried-over rows are present immediately (not blank)")
	require.Equal(t, 2, ns.Table.GetCurrItem(),
		"cursor must carry over on refresh, not flash to the top row")
	require.NotNil(t, ns.GetCurrRow())
	require.Equal(t, wantURL, ns.GetCurrRow().GetUrl(),
		"the same issue must stay selected across the refresh")
}
