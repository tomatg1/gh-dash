package tui

import (
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prrow"
)

func previewShows(m Model, num int) bool {
	return strings.Contains(m.View().Content, "· #"+strconv.Itoa(num))
}

func currURL(m Model) string { return m.getCurrSection().GetCurrRow().GetUrl() }

// Scrolling through rows must not do, for every row passed, the work that only
// the row you land on needs: rendering its preview (glamour markdown), fetching
// its details, writing the selection to disk. That work waits for the selection
// to settle -- while the STATE that keys act on follows the cursor immediately,
// so nothing pressed right after scrolling can act on the previous row.
func TestScroll_DefersRowWorkUntilTheSelectionSettles(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	m := perfModel(t, 30, 30, 1)
	m.layoutStateKey = "instance:settle"
	down := tea.MouseWheelMsg{X: 20, Y: 20, Button: tea.MouseWheelUp}

	m = step(m, down)
	m = step(m, rowSettledMsg{seq: m.rowSettleSeq})
	start := selectedNumber(m)
	require.True(t, previewShows(m, start), "a settled selection shows its preview")

	for range 5 {
		m = step(m, down)
	}
	landed := selectedNumber(m)
	require.NotEqual(t, start, landed)

	require.True(t, previewShows(m, start), "no preview is rendered for rows scrolled past")
	require.NotEqual(t, currURL(m), loadSelection("instance:settle", m.getCurrSection().GetConfig().Title),
		"the selection is not written to disk for every row scrolled past")
	require.True(t, m.prView.SetEnrichedPR(m.getCurrSection().GetCurrRow().(*prrow.Data).Enriched),
		"but the preview's STATE already points at the landed row, so keys act on it")

	m = step(m, rowSettledMsg{seq: m.rowSettleSeq - 1})
	require.True(t, previewShows(m, start), "a settle superseded by a later scroll does nothing")

	m = step(m, rowSettledMsg{seq: m.rowSettleSeq})
	require.True(t, previewShows(m, landed), "once settled, the landed row's preview is rendered")
	require.Equal(t, currURL(m), loadSelection("instance:settle", m.getCurrSection().GetConfig().Title),
		"and the selection is remembered")
}

// settleMsgs executes a command tree the way bubbletea would, but only keeps the
// settle messages, which is all this test is about. Other commands (fetches,
// browser opens) are not run.
func settleMsgs(cmd tea.Cmd) []rowSettledMsg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	switch msg := msg.(type) {
	case tea.BatchMsg:
		var out []rowSettledMsg
		for _, c := range msg {
			if c != nil && isTimer(c) {
				out = append(out, settleMsgs(c)...)
			}
		}

		return out
	case rowSettledMsg:
		return []rowSettledMsg{msg}
	}

	return nil
}

// isTimer runs only commands that return within the settle delay without side
// effects worth worrying about; anything else is skipped by timing out.
func isTimer(c tea.Cmd) bool {
	done := make(chan tea.Msg, 1)
	go func() { done <- c() }()
	select {
	case m := <-done:
		_, ok := m.(rowSettledMsg)
		_, batch := m.(tea.BatchMsg)
		return ok || batch
	case <-time.After(3 * rowSettleDelay):
		return false
	}
}

// End to end, with the real timer commands: a burst of scrolling settles once,
// shortly after it stops, and shows the landed row's preview. A broken re-arm
// would leave the preview stuck on the row before the scroll.
func TestScroll_SettlesOnceAfterAScrollBurst(t *testing.T) {
	m := perfModel(t, 30, 30, 1)
	down := tea.MouseWheelMsg{X: 20, Y: 20, Button: tea.MouseWheelUp}

	var pending []tea.Cmd
	for range 6 {
		next, cmd := m.Update(down)
		m = next.(Model)
		pending = append(pending, cmd)
	}
	landed := selectedNumber(m)

	settles := 0
	for len(pending) > 0 && settles < 10 {
		cmd := pending[0]
		pending = pending[1:]
		for _, msg := range settleMsgs(cmd) {
			next, follow := m.Update(msg)
			m = next.(Model)
			settles++
			pending = append(pending, follow)
		}
	}

	require.False(t, m.settleArmed, "the burst must end settled")
	require.LessOrEqual(t, settles, 2, "one timer for the whole burst, not one per scroll event")
	require.True(t, previewShows(m, landed), "the settled preview shows the landed row")
}
