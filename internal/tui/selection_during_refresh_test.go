package tui

import (
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/data"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prrow"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prssection"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/constants"
)

// landFetch delivers a section's in-flight fetch result through the real path.
func landFetch(t *testing.T, m Model, prs []prrow.Data) Model {
	t.Helper()
	ps, ok := m.getCurrSection().(*prssection.Model)
	require.True(t, ok)
	next, _ := m.Update(constants.TaskFinishedMsg{
		SectionId: ps.Id, SectionType: prssection.SectionType, TaskId: ps.LastFetchTaskId,
		Msg: prssection.SectionPullRequestsFetchedMsg{
			Prs: prs, TotalCount: len(prs), TaskId: ps.LastFetchTaskId,
			PageInfo: data.PageInfo{HasNextPage: false},
		},
	})

	return next.(Model)
}

func selectedNumber(m Model) int { return m.getCurrSection().GetCurrRow().GetNumber() }

// Scrolling while a refresh is in flight is the user choosing a new row. When
// the fetch lands it must not snap the selection back to where it was when the
// refresh started.
func TestRefreshLanding_KeepsWhereTheUserScrolledTo(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	m := perfModel(t, 20, 20, 1)
	down := tea.MouseWheelMsg{X: 20, Y: 20, Button: tea.MouseWheelUp}

	m = step(m, down)
	m = step(m, down)
	m = step(m, intervalRefresh(time.Now())) // refresh starts on row 2
	for range 3 {
		m = step(m, down) // ...and the user keeps scrolling while it's in flight
	}
	chosen := selectedNumber(m)

	m = landFetch(t, m, perfPRs(1, 20))

	require.Equal(t, chosen, selectedNumber(m),
		"the landing fetch must not snap back to the row selected when the refresh started")
}

// Same, when the fresh data comes back in a different order: the user's choice
// is a PR, not a position, so it must be found by identity.
func TestRefreshLanding_KeepsTheUsersChoiceWhenRowsReorder(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	m := perfModel(t, 20, 20, 1)
	down := tea.MouseWheelMsg{X: 20, Y: 20, Button: tea.MouseWheelUp}

	m = step(m, down)
	m = step(m, intervalRefresh(time.Now()))
	for range 4 {
		m = step(m, down)
	}
	chosen := selectedNumber(m)

	reordered := perfPRs(1, 20)
	slices.Reverse(reordered)
	m = landFetch(t, m, reordered)

	require.Equal(t, chosen, selectedNumber(m),
		"the user's chosen PR must stay selected even when the fresh rows are reordered")
}
