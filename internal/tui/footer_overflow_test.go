package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/tui/common"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/footer"
)

// footerVisible reports whether the rendered frame fits the terminal and the
// "? help" toggle lands within the visible row budget (the bottom of an
// alt-screen is clipped, not scrolled, so an over-tall frame hides the footer).
func footerVisible(t *testing.T, m Model) (int, bool) {
	t.Helper()
	content := m.View().Content
	lines := strings.Split(content, "\n")
	visible := lines
	if len(lines) > m.ctx.ScreenHeight {
		visible = lines[:m.ctx.ScreenHeight]
	}
	return lipgloss.Height(content), strings.Contains(strings.Join(visible, "\n"), "? help")
}

// Regression: dragging the divider high shrinks the list toward zero, but the
// list section can't render below common.MinListHeight. Before the fix the
// preview was allowed to take all but one row, so the list overflowed its
// allotment and pushed the footer -- and the "? help" toggle -- off the bottom.
// This got worse the instant help expanded, because expanding help steals ~16
// rows from the content area and the clamp took them all out of the list.
func TestFooter_StaysVisibleWithHighSeparatorAndHelp(t *testing.T) {
	zone.NewGlobal()
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	m := bottomPreviewModel(t)
	screen := m.ctx.ScreenHeight

	// Drag the divider as high as it goes (biggest possible preview).
	m.setPreviewHeightFromSeparatorY(2)
	require.GreaterOrEqual(t, m.ctx.MainContentHeight, common.MinListHeight,
		"even a max drag must leave the list its minimum renderable height")

	h, ok := footerVisible(t, m)
	require.LessOrEqual(t, h, screen, "help closed: frame must fit the screen")
	require.True(t, ok, "help closed: the footer must stay visible")

	// Now expand help via the same path a footer click uses.
	m.footer.ShowAll = true
	m.syncMainContentDimensions()
	m.syncProgramContext()
	require.GreaterOrEqual(t, m.ctx.MainContentHeight, common.MinListHeight,
		"expanding help must not squeeze the list below its minimum")

	h, ok = footerVisible(t, m)
	require.LessOrEqual(t, h, screen, "help open: frame must fit the screen")
	require.True(t, ok, "help open: the footer must stay visible")

	_ = footer.ZoneHelp
	_ = tea.MouseLeft
}
