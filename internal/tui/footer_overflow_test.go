package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/tui/common"
)

// statusBarVisible reports whether the render fits the terminal and the footer
// status bar lands within the visible rows. The alt-screen clips its bottom, so
// an over-tall frame hides the status bar. We look for the "donate" indicator,
// which appears only in the status bar -- NOT for "? help", which the expanded
// help keymap also lists as a binding (that collision hid this bug once already).
func statusBarVisible(t *testing.T, m Model) (int, bool) {
	t.Helper()
	content := m.View().Content
	lines := strings.Split(content, "\n")
	visible := lines
	if len(lines) > m.ctx.ScreenHeight {
		visible = lines[:m.ctx.ScreenHeight]
	}
	return lipgloss.Height(content), strings.Contains(strings.Join(visible, "\n"), "donate")
}

// Regression: dragging the divider high shrinks the list toward zero, but the
// list section can't render below common.MinListHeight. The layout let the
// preview take all but one row, so the list overflowed its allotment and pushed
// the footer -- and the "? help" toggle -- off the bottom. Expanding help made it
// worse and easier to hit: it steals ~16 rows from the content area, and the
// clamp took them all out of the list.
//
// Driven the way the real app runs: a WindowSizeMsg (so the help/footer width is
// set and the section renders its true styled minimum) and a preview tall enough
// to actually fill its allotment. An empty preview under-fills and hides the
// overflow.
func TestFooter_StaysVisibleWithHighSeparatorAndHelp(t *testing.T) {
	zone.NewGlobal()
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	m := bottomPreviewModel(t)
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 125, Height: 52})
	m = upd.(Model)
	screen := m.ctx.ScreenHeight

	// A preview tall enough to fill whatever height it's given.
	m.sidebar.SetContent(strings.Repeat("preview line\n", 200))

	// Drag the divider as high as it goes (largest possible preview).
	m.setPreviewHeightFromSeparatorY(2)
	m.syncProgramContext()
	require.GreaterOrEqual(t, m.ctx.MainContentHeight, common.MinListHeight,
		"even a max drag must leave the list its minimum renderable height")

	h, ok := statusBarVisible(t, m)
	require.LessOrEqual(t, h, screen, "help closed: frame must fit the screen")
	require.True(t, ok, "help closed: the status bar must stay visible")

	// Expand help the way a footer click / '?' does.
	m.footer.ShowAll = true
	m.syncMainContentDimensions()
	m.syncProgramContext()
	require.GreaterOrEqual(t, m.ctx.MainContentHeight, common.MinListHeight,
		"expanding help must not squeeze the list below its minimum")

	h, ok = statusBarVisible(t, m)
	require.LessOrEqual(t, h, screen, "help open: frame must fit the screen")
	require.True(t, ok, "help open: the status bar must stay visible")
}
