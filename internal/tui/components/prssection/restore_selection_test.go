package prssection

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/data"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prrow"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/theme"
)

func pr(number int, url string) prrow.Data {
	return prrow.Data{Primary: &data.PullRequestData{Number: number, Url: url, State: "OPEN"}}
}

func newRenderableModel(t *testing.T) Model {
	t.Helper()
	cfg, err := config.ParseConfig(config.Location{
		ConfigFlag:       "../../../config/testdata/test-config.yml",
		SkipGlobalConfig: true,
	})
	require.NoError(t, err)
	ctx := &context.ProgramContext{Config: &cfg, ScreenWidth: 160, ScreenHeight: 40}
	ctx.Theme = theme.ParseTheme(&cfg)
	ctx.Styles = context.InitStyles(ctx.Theme)
	return NewModel(1, ctx, config.PrsSectionConfig{Title: "T", Filters: "is:open"}, time.Now(), time.Now())
}

// The selected PR must stay selected across a refresh even when the list is
// REORDERED (the whole point of keying on URL, not index).
func TestRestoreSelection_FollowsPRAcrossReorder(t *testing.T) {
	m := newRenderableModel(t)
	m.Prs = []prrow.Data{pr(1, "u1"), pr(2, "u2"), pr(3, "u3")}
	m.Table.SetRows(m.BuildRows())
	m.Table.SetCurrItem(1) // select PR #2 (u2), at index 1
	require.Equal(t, 1, m.Table.GetCurrItem())

	// refresh: stash the selection, then fresh data arrives reordered (#2 first)
	if r := m.GetCurrRow(); r != nil {
		m.SetPendingSelection(r.GetUrl())
	}
	m.Prs = []prrow.Data{pr(2, "u2"), pr(1, "u1"), pr(3, "u3")}
	m.Table.SetRows(m.BuildRows())
	m.restoreSelection()

	require.Equal(t, 0, m.Table.GetCurrItem(), "selection should follow PR #2 to its new index 0")
}

// If the selected PR is gone after refresh (merged/closed), the cursor stays put.
func TestRestoreSelection_GonePRLeavesCursor(t *testing.T) {
	m := newRenderableModel(t)
	m.Prs = []prrow.Data{pr(1, "u1"), pr(2, "u2")}
	m.Table.SetRows(m.BuildRows())
	m.Table.SetCurrItem(1)

	m.SetPendingSelection("u2")
	m.Prs = []prrow.Data{pr(1, "u1")} // #2 merged away
	m.Table.SetRows(m.BuildRows())
	m.restoreSelection()

	require.Equal(t, 0, m.Table.GetCurrItem(), "cursor clamps to a valid row, no panic")
}

// No pending selection => restore is a no-op (normal navigation isn't disturbed).
func TestRestoreSelection_NoPendingIsNoop(t *testing.T) {
	m := newRenderableModel(t)
	m.Prs = []prrow.Data{pr(1, "u1"), pr(2, "u2"), pr(3, "u3")}
	m.Table.SetRows(m.BuildRows())
	m.Table.SetCurrItem(2)

	m.restoreSelection() // nothing pending
	require.Equal(t, 2, m.Table.GetCurrItem(), "no pending selection must not move the cursor")
}
