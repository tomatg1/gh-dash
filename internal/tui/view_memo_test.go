package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prssection"
)

// frame is the rendered screen minus the footer's status line, whose "Updated
// ~1s ago" (and the alignment after it) depends on when each model was built --
// not on anything a memo could get wrong. The footer is not memoized.
func frame(m Model) string {
	lines := strings.Split(m.View().Content, "\n")
	kept := lines[:0]
	for _, l := range lines {
		if !strings.Contains(l, "Updated ") {
			kept = append(kept, l)
		}
	}

	return strings.Join(kept, "\n")
}

func scrollOf(m Model) [2]int {
	top, curr := m.getCurrSection().(*prssection.Model).Table.ScrollState()

	return [2]int{top, curr}
}

// Views are memoized: each component reuses its last frame while the inputs it
// depends on are unchanged, so a dependency missing from a memo's key shows up
// as a stale frame. The model under test renders after every step, keeping its
// memos warm. At each checkpoint it is compared with a model that has NEVER
// rendered -- built fresh and replayed through the same steps -- whose memos are
// empty, so its frame is a true from-scratch render.
func TestViewMemos_NeverServeAStaleFrame(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	warm := perfModel(t, 30, 30, 1)

	down := tea.MouseWheelMsg{X: 20, Y: 20, Button: tea.MouseWheelUp}
	up := tea.MouseWheelMsg{X: 20, Y: 20, Button: tea.MouseWheelDown}
	settle := func(m Model) tea.Msg { return rowSettledMsg{seq: m.rowSettleSeq} }

	type stepFn func(m Model) tea.Msg
	phases := []struct {
		name  string
		steps []stepFn
	}{
		{"scrolling past a page and back", func() []stepFn {
			var st []stepFn
			for range 18 {
				st = append(st, func(Model) tea.Msg { return down })
			}
			for range 3 {
				st = append(st, func(Model) tea.Msg { return up })
			}

			return append(st, func(Model) tea.Msg { return down })
		}()},
		{"settling, which renders the preview", []stepFn{settle}},
		{"scrolling the preview", []stepFn{
			func(Model) tea.Msg { return tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl} },
		}},
		{"typing a filter", []stepFn{
			func(Model) tea.Msg { return tea.KeyPressMsg{Text: "/"} },
			func(Model) tea.Msg { return tea.KeyPressMsg{Text: "x"} },
			func(Model) tea.Msg { return tea.KeyPressMsg{Code: tea.KeyEscape} },
		}},
		{"resizing", []stepFn{
			func(Model) tea.Msg { return tea.WindowSizeMsg{Width: 150, Height: 44} },
		}},
		{"closing and reopening the preview", []stepFn{
			func(Model) tea.Msg { return tea.KeyPressMsg{Text: "p"} },
			func(Model) tea.Msg { return tea.KeyPressMsg{Text: "p"} },
		}},
		{"a refresh landing", []stepFn{
			func(Model) tea.Msg { return intervalRefresh(time.Now()) },
		}},
	}

	var done []stepFn
	fresh := func() Model {
		m := perfModel(t, 30, 30, 1)
		for _, s := range done {
			next, _ := m.Update(s(m)) // never rendered: no memo is warm
			m = next.(Model)
		}

		return m
	}

	for _, ph := range phases {
		for _, s := range ph.steps {
			warm = step(warm, s(warm))
			done = append(done, s)
		}
		f := fresh()
		if fv, wv := frame(f), frame(warm); fv != wv {
			fl, wl := strings.Split(fv, "\n"), strings.Split(wv, "\n")
			for i := range min(len(fl), len(wl)) {
				if fl[i] != wl[i] {
					t.Logf("line %d\n fresh: %q\n warm : %q", i, ansi.Strip(fl[i]), ansi.Strip(wl[i]))
				}
			}
			t.Logf("lines fresh=%d warm=%d  sel fresh=%d warm=%d  scroll fresh=%v warm=%v", len(fl), len(wl),
				selectedNumber(f), selectedNumber(warm), scrollOf(f), scrollOf(warm))
		}
		require.Equal(t, frame(f), frame(warm), "after "+ph.name)
		// Staleness both models share is invisible to the comparison above, so
		// also hold every tab's cells to a fresh rebuild: they are formatted for
		// the current widths and selection, whatever the layout went through.
		for i, sec := range warm.prs {
			if ps, ok := sec.(*prssection.Model); ok {
				require.Equal(t, ps.BuildRows(), ps.Table.Rows, "tab %d's cells after %s", i, ph.name)
			}
		}
	}
}
