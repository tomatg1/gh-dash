package tui

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

// selectedScreenLine is where the selected PR's row sits on screen, looking only
// at the list column (the preview pane repeats the number in its header).
func selectedScreenLine(t *testing.T, m Model) int {
	t.Helper()
	num := "#" + strconv.Itoa(m.getCurrSection().GetCurrRow().GetNumber())
	for i, l := range strings.Split(m.View().Content, "\n") {
		if strings.Contains(ansi.Truncate(ansi.Strip(l), m.ctx.MainContentWidth, ""), num) {
			return i
		}
	}
	t.Fatalf("selected row %s not on screen", num)

	return -1
}

// Reversing scroll direction is what a trackpad does constantly. After coming
// back up a few rows, scrolling down again must walk the highlight down the
// screen until it reaches the bottom edge -- not freeze it mid-screen and slide
// the list underneath, which is what a rebuild resetting the viewport's window
// used to cause on every message.
func TestScroll_HighlightWalksBackDownAfterAReversal(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	m := perfModel(t, 60, 60, 1)
	down := tea.MouseWheelMsg{X: 20, Y: 20, Button: tea.MouseWheelUp} // natural scrolling
	up := tea.MouseWheelMsg{X: 20, Y: 20, Button: tea.MouseWheelDown}

	for range 30 {
		m = step(m, down)
	}
	for range 4 {
		m = step(m, up)
	}
	m = step(m, spinner.TickMsg{}) // a repaint in between, as a fetch would cause

	prev := selectedScreenLine(t, m)
	for i := range 3 {
		m = step(m, down)
		line := selectedScreenLine(t, m)
		require.Greater(t, line, prev,
			"scroll-down %d after a reversal: the highlight must move down the screen, not freeze", i+1)
		prev = line
	}
}

// A refresh rebuilds every section from scratch. Restoring the cursor INDEX is
// not enough: a fresh viewport walked down to that index leaves the selected row
// at the bottom edge, wherever it was before -- so the list jumps under a
// selection that didn't change. The row must stay on the same screen line, both
// when the refresh starts and when its data lands.
func TestRefresh_KeepsTheSelectedRowOnTheSameScreenLine(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	m := perfModel(t, 60, 60, 1)
	down := tea.MouseWheelMsg{X: 20, Y: 20, Button: tea.MouseWheelUp}
	up := tea.MouseWheelMsg{X: 20, Y: 20, Button: tea.MouseWheelDown}

	for range 30 {
		m = step(m, down)
	}
	for range 4 {
		m = step(m, up) // mid-screen, with the list scrolled
	}
	line := selectedScreenLine(t, m)

	m = step(m, intervalRefresh(time.Now()))
	require.Equal(t, line, selectedScreenLine(t, m), "the refresh starting must not move the list")

	m = landFetch(t, m, perfPRs(1, 60))
	require.Equal(t, line, selectedScreenLine(t, m), "the refresh landing must not move the list")
}
