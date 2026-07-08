package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/footer"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prview"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/table"
)

// ── footer: help toggle ──────────────────────────────────────────────────────

func TestFooterClick_HelpTogglesFullHelp(t *testing.T) {
	m, _ := zoneTestModel(t)
	_ = m.View()
	z := waitForZone(t, footer.ZoneHelp)

	require.False(t, m.footer.ShowAll, "help starts collapsed")

	m = click(t, m, z.StartX+1, z.StartY)
	require.True(t, m.footer.ShowAll, "clicking '? help' expands the full help")

	// It's a toggle.
	_ = m.View()
	z = waitForZone(t, footer.ZoneHelp)
	m = click(t, m, z.StartX+1, z.StartY)
	require.False(t, m.footer.ShowAll, "clicking again collapses it")
}

// ── footer: view switcher ────────────────────────────────────────────────────

func TestFooterClick_ViewSwitcherSwitchesView(t *testing.T) {
	m, _ := zoneTestModel(t)
	require.Equal(t, config.PRsView, m.ctx.View, "starts in PRs")

	_ = m.View()
	issues := waitForZone(t, footer.ViewZoneID(config.IssuesView))
	m = click(t, m, issues.StartX+1, issues.StartY)

	require.Equal(t, config.IssuesView, m.ctx.View, "clicking Issues switches the view")

	_ = m.View()
	prs := waitForZone(t, footer.ViewZoneID(config.PRsView))
	m = click(t, m, prs.StartX+1, prs.StartY)
	require.Equal(t, config.PRsView, m.ctx.View, "clicking PRs switches back")
}

// Clicking the already-active view button does nothing (and must not crash).
func TestFooterClick_ActiveViewIsNoOp(t *testing.T) {
	m, _ := zoneTestModel(t)
	_ = m.View()
	prs := waitForZone(t, footer.ViewZoneID(config.PRsView))

	m = click(t, m, prs.StartX+1, prs.StartY)
	require.Equal(t, config.PRsView, m.ctx.View)
}

// ── preview detail tabs ──────────────────────────────────────────────────────

// previewOpenModel builds the PRs view with the sidebar open on a PR, so the
// Overview/Activity/Commits/Checks/Files tabs are on screen.
func previewOpenModel(t *testing.T) Model {
	t.Helper()

	m, _ := zoneTestModel(t)
	m.sidebar.IsOpen = true
	m.sidebar.UpdateProgramContext(m.ctx)
	m.syncMainContentDimensions()
	m.syncProgramContext()
	// select a row and push its PR into the preview
	m.prs[0].SetCurrRow(0)
	m.syncSidebar()

	return m
}

func TestPreviewTabClick_SwitchesDetailTab(t *testing.T) {
	// One hop per fresh model: selecting a tab re-centers the carousel, which
	// shifts every tab's x-position, so chaining clicks would use stale coords.
	for _, idx := range []int{1, 2, 3} {
		t.Run(prview.TabZoneID(idx), func(t *testing.T) {
			m := previewOpenModel(t)
			_ = m.View()
			require.Equal(t, 0, m.prView.SelectedTabIdx(), "starts on Overview")

			z := waitForZone(t, prview.TabZoneID(idx))
			m = click(t, m, z.StartX+1, z.StartY)

			require.Equal(t, idx, m.prView.SelectedTabIdx(),
				"clicking preview tab %d should select it", idx)
		})
	}
}

// A preview-tab zone must not be clickable when the preview is closed.
func TestPreviewTabClick_IgnoredWhenSidebarClosed(t *testing.T) {
	m, _ := zoneTestModel(t)
	require.False(t, m.sidebar.IsOpen)
	_ = m.View()
	waitForZone(t, table.RowZoneID(0))

	// Even if a stale zone existed, a click while closed must be inert: the row
	// under the click is what should respond, not a preview tab.
	before := m.prView.SelectedTabIdx()
	updated, _ := m.Update(tea.MouseClickMsg{X: 0, Y: 39, Button: tea.MouseLeft})
	m = updated.(Model)
	updated, _ = m.Update(tea.MouseReleaseMsg{X: 0, Y: 39, Button: tea.MouseLeft})
	m = updated.(Model)

	require.Equal(t, before, m.prView.SelectedTabIdx())
}
