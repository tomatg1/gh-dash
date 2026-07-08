package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/data"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/common"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/branchsidebar"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/footer"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/issueview"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/notificationview"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prrow"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prssection"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prview"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/section"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/sidebar"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/table"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/tabs"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/keys"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/theme"
)

// waitForZone polls: bubblezone's Scan hands the scanned zones to a worker
// goroutine over a channel, so a zone is not queryable the instant Scan returns.
func waitForZone(t *testing.T, id string) *zone.ZoneInfo {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if z := zone.Get(id); !z.IsZero() {
			return z
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("zone %q never registered after rendering a frame", id)
	return nil
}

// zoneTestModel builds a PRs view with two rows and one tab, wide enough that
// nothing is truncated away. The returned slice records every task started,
// which is how we detect that a click actually opened a PR.
func zoneTestModel(t *testing.T) (Model, *[]string) {
	t.Helper()

	zone.NewGlobal()
	zone.SetEnabled(true)
	t.Cleanup(func() { zone.SetEnabled(false) })

	cfg, err := config.ParseConfig(config.Location{
		ConfigFlag:       "../config/testdata/test-config.yml",
		SkipGlobalConfig: true,
	})
	require.NoError(t, err)

	started := &[]string{}
	ctx := &context.ProgramContext{
		Config:       &cfg,
		ScreenWidth:  160,
		ScreenHeight: 40,
		View:         config.PRsView,
		StartTask: func(task context.Task) tea.Cmd {
			*started = append(*started, task.Id)
			return nil
		},
	}
	ctx.Theme = theme.ParseTheme(ctx.Config)
	ctx.Styles = context.InitStyles(ctx.Theme)

	prSection := prssection.NewModel(
		0,
		ctx,
		config.PrsSectionConfig{Title: "Mine", Filters: "is:open"},
		time.Now(),
		time.Now(),
	)
	prSection.Prs = []prrow.Data{
		{Primary: &data.PullRequestData{
			Number: 1, Title: "first pr", State: "OPEN",
			Url: "https://github.com/o/r/pull/1",
		}},
		{Primary: &data.PullRequestData{
			Number: 2, Title: "second pr", State: "OPEN",
			Url: "https://github.com/o/r/pull/2",
		}},
	}
	prSection.Table.SetRows(prSection.BuildRows())

	m := Model{
		ctx:              ctx,
		keys:             keys.Keys,
		prs:              []section.Section{&prSection},
		sidebar:          sidebar.NewModel(),
		footer:           footer.NewModel(ctx),
		tabs:             tabs.NewModel(ctx),
		prView:           prview.NewModel(ctx),
		issueSidebar:     issueview.NewModel(ctx),
		branchSidebar:    branchsidebar.NewModel(ctx),
		notificationView: notificationview.NewModel(ctx),
		frameBuf:         new(string),
		lastClickRow:     -1,
	}
	m.tabs.SetSections(m.prs)
	m.syncMainContentDimensions()
	m.syncProgramContext()

	return m, started
}

func openedBrowser(started *[]string) bool {
	for _, id := range *started {
		if strings.HasPrefix(id, "open_browser") {
			return true
		}
	}

	return false
}

// The tab title is marked before it enters the carousel, which runs
// lipgloss.Width and ansi.Truncate over it. Those must leave bubblezone's
// private CSI markers intact or the tab is silently unclickable.
func TestMouseZones_TabSurvivesCarouselTruncation(t *testing.T) {
	m, _ := zoneTestModel(t)

	_ = m.View() // View() runs zone.Scan over the composed frame

	tab := waitForZone(t, tabs.TabZoneID(0))
	require.False(t, tab.IsZero(), "tab zone should register")
}

func TestMouseZones_RowsRegisterInOrder(t *testing.T) {
	m, _ := zoneTestModel(t)

	_ = m.View()

	row0 := waitForZone(t, table.RowZoneID(0))
	row1 := waitForZone(t, table.RowZoneID(1))

	require.Less(t, row0.StartY, row1.StartY, "row 0 should render above row 1")
}

// A press is not a click. Until the button comes back up we cannot know whether
// the user is clicking or starting a drag-selection, so nothing may happen yet.
func TestMousePress_AloneDoesNothing(t *testing.T) {
	m, started := zoneTestModel(t)
	_ = m.View()
	row1 := waitForZone(t, table.RowZoneID(1))

	updated, _ := m.Update(tea.MouseClickMsg{
		X: row1.StartX + 1, Y: row1.StartY, Button: tea.MouseLeft,
	})
	m = updated.(Model)

	require.Equal(t, 0, m.prs[0].CurrRow(), "press must not move the selection")
	require.False(t, openedBrowser(started), "press must not open a browser")
	require.True(t, m.mouseDown, "press should arm a potential drag")
}

// click sends a press+release on the same cell, i.e. a plain click.
func click(t *testing.T, m Model, x, y int) Model {
	t.Helper()

	updated, _ := m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	m = updated.(Model)
	updated, _ = m.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})

	return updated.(Model)
}

// The refinement: a bare click selects the row but must NOT open a browser.
func TestMouseClick_SingleClickSelectsWithoutOpening(t *testing.T) {
	m, started := zoneTestModel(t)
	_ = m.View()
	row1 := waitForZone(t, table.RowZoneID(1))

	m = click(t, m, row1.StartX+1, row1.StartY)

	require.Equal(t, 1, m.prs[0].CurrRow(), "click should select the clicked row")
	require.False(t, openedBrowser(started), "a bare click must not open a browser")
	require.False(t, m.sel.active, "a click leaves no selection behind")
}

// Two clicks on the same row inside the window open it.
func TestMouseClick_DoubleClickOpensRow(t *testing.T) {
	m, started := zoneTestModel(t)

	base := time.Unix(1700000000, 0)
	now := base
	orig := nowFunc
	nowFunc = func() time.Time { return now }
	t.Cleanup(func() { nowFunc = orig })

	_ = m.View()
	row1 := waitForZone(t, table.RowZoneID(1))
	x, y := row1.StartX+1, row1.StartY

	m = click(t, m, x, y)
	require.False(t, openedBrowser(started), "first click only selects")

	now = base.Add(100 * time.Millisecond)
	m = click(t, m, x, y)

	require.Equal(t, 1, m.prs[0].CurrRow())
	require.True(t, openedBrowser(started), "double-click should open the PR")
}

// Clicking the "#1234" opens the PR on the first click, no drag, no pairing.
func TestMouseClick_OnPRNumberOpensImmediately(t *testing.T) {
	m, started := zoneTestModel(t)
	_ = m.View()

	num := waitForZone(t, common.RowTargetZoneID(1, common.ZoneNumber))
	m = click(t, m, num.StartX, num.StartY)

	require.Equal(t, 1, m.prs[0].CurrRow(), "clicking the number also selects the row")
	require.True(t, openedBrowser(started), "clicking the number opens the PR")
}

// Icon cells are individually clickable. The exact sub-page URL is pinned by
// TestRowClickAction_IconsOpenTheirSubPages; here we prove the zone dispatches.
func TestMouseClick_OnIconsOpens(t *testing.T) {
	for _, tc := range []struct{ name, target string }{
		{"ci", common.ZoneCi},
		{"review", common.ZoneReview},
		{"comments", common.ZoneComments},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, started := zoneTestModel(t)
			_ = m.View()

			z := waitForZone(t, common.RowTargetZoneID(1, tc.target))
			m = click(t, m, z.StartX, z.StartY)

			require.Equal(t, 1, m.prs[0].CurrRow(), "icon click selects its row")
			require.True(t, openedBrowser(started), "icon click opens a page")
		})
	}
}

// An icon click must not arm a double-click pairing on the row.
func TestMouseClick_IconDoesNotArmDoubleClick(t *testing.T) {
	m, _ := zoneTestModel(t)
	_ = m.View()

	z := waitForZone(t, common.RowTargetZoneID(1, common.ZoneCi))
	m = click(t, m, z.StartX, z.StartY)

	require.Equal(t, -1, m.lastClickRow, "clicking an icon leaves no pending pair")
}

// dragClick sends press, a motion to (x+dx, y+dy), then release there.
func dragClick(t *testing.T, m Model, x, y, dx, dy int) (Model, string) {
	t.Helper()

	var copied string
	orig := copyToClipboard
	copyToClipboard = func(s string) error { copied = s; return nil }
	t.Cleanup(func() { copyToClipboard = orig })

	up, _ := m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	m = up.(Model)
	up, _ = m.Update(tea.MouseMotionMsg{X: x + dx, Y: y + dy, Button: tea.MouseLeft})
	m = up.(Model)
	up, _ = m.Update(tea.MouseReleaseMsg{X: x + dx, Y: y + dy, Button: tea.MouseLeft})

	return up.(Model), copied
}

// The reported bug: a trackpad tap jitters a cell or two, and that used to
// register as a drag -- copying a stray character and leaving a lingering
// highlight instead of selecting the row.
func TestMouseClick_SloppyClickStillSelects(t *testing.T) {
	for _, tc := range []struct {
		name   string
		dx, dy int
	}{
		{"1 cell right", 1, 0},
		{"2 cells right", 2, 0},
		{"1 cell down", 0, 1},
		{"1 diagonal", 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, started := zoneTestModel(t)
			_ = m.View()
			row := waitForZone(t, table.RowZoneID(1))

			m, copied := dragClick(t, m, row.StartX+10, row.StartY, tc.dx, tc.dy)

			require.False(t, m.sel.active, "no lingering selection after a sloppy click")
			require.Empty(t, copied, "a sloppy click must not copy anything")
			require.False(t, openedBrowser(started), "a single sloppy click only selects")
			require.Equal(t, 1, m.prs[0].CurrRow(), "the clicked row is selected")
		})
	}
}

// Past the threshold it is a real drag: it selects and copies.
func TestMouseDrag_PastThresholdSelects(t *testing.T) {
	m, started := zoneTestModel(t)
	_ = m.View()
	row := waitForZone(t, table.RowZoneID(0))

	m, copied := dragClick(t, m, 0, row.StartY, 40, 0) // well past dragThresholdX

	require.True(t, m.sel.active, "a real drag shows a selection")
	require.NotEmpty(t, copied, "a real drag copies")
	require.False(t, openedBrowser(started), "a drag never opens")
}

// A drag selects text. It must copy, and it must never open a browser.
func TestMouseDrag_SelectsTextAndNeverOpens(t *testing.T) {
	m, started := zoneTestModel(t)

	var copied string
	orig := copyToClipboard
	copyToClipboard = func(s string) error { copied = s; return nil }
	t.Cleanup(func() { copyToClipboard = orig })

	_ = m.View()
	row0 := waitForZone(t, table.RowZoneID(0))

	// press at the row's top-left, drag across and down to its bottom-right
	updated, _ := m.Update(tea.MouseClickMsg{X: 0, Y: row0.StartY, Button: tea.MouseLeft})
	m = updated.(Model)
	updated, _ = m.Update(tea.MouseMotionMsg{X: 150, Y: row0.EndY, Button: tea.MouseLeft})
	m = updated.(Model)
	require.True(t, m.dragged, "motion while held is a drag")
	require.True(t, m.sel.active, "drag should show a selection")

	updated, _ = m.Update(tea.MouseReleaseMsg{X: 150, Y: row0.EndY, Button: tea.MouseLeft})
	m = updated.(Model)

	require.False(t, openedBrowser(started), "a drag must never open a browser")
	require.Equal(t, 0, m.prs[0].CurrRow(), "a drag must not move the selection")
	require.Contains(t, copied, "first", "the dragged row's text should be copied")
	require.True(t, m.sel.active, "the highlight stays until the next click or key")
}

// Dragging back onto the press cell is still a drag, not a click.
func TestMouseDrag_ReturningToAnchorDoesNotOpen(t *testing.T) {
	m, started := zoneTestModel(t)
	_ = m.View()
	row1 := waitForZone(t, table.RowZoneID(1))

	x, y := row1.StartX+1, row1.StartY
	updated, _ := m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	m = updated.(Model)
	updated, _ = m.Update(tea.MouseMotionMsg{X: x + 5, Y: y, Button: tea.MouseLeft})
	m = updated.(Model)
	updated, _ = m.Update(tea.MouseMotionMsg{X: x, Y: y, Button: tea.MouseLeft})
	m = updated.(Model)
	updated, _ = m.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	m = updated.(Model)

	require.False(t, openedBrowser(started), "a drag that returns home must not open")
	require.Equal(t, 0, m.prs[0].CurrRow())
}

func TestMouseClick_OutsideAnyZoneIsInert(t *testing.T) {
	m, started := zoneTestModel(t)
	_ = m.View()
	waitForZone(t, table.RowZoneID(0))

	updated, _ := m.Update(tea.MouseClickMsg{X: 0, Y: 39, Button: tea.MouseLeft})
	m = updated.(Model)
	updated, _ = m.Update(tea.MouseReleaseMsg{X: 0, Y: 39, Button: tea.MouseLeft})
	m = updated.(Model)

	require.Equal(t, 0, m.prs[0].CurrRow(), "selection should not move")
	require.False(t, openedBrowser(started), "nothing should open")
}

func TestMouseClick_NonLeftButtonIgnored(t *testing.T) {
	m, started := zoneTestModel(t)
	_ = m.View()
	row1 := waitForZone(t, table.RowZoneID(1))

	updated, _ := m.Update(tea.MouseClickMsg{
		X: row1.StartX + 1, Y: row1.StartY, Button: tea.MouseRight,
	})
	m = updated.(Model)
	updated, _ = m.Update(tea.MouseReleaseMsg{
		X: row1.StartX + 1, Y: row1.StartY, Button: tea.MouseRight,
	})
	m = updated.(Model)

	require.Equal(t, 0, m.prs[0].CurrRow(), "right-click should not select")
	require.False(t, openedBrowser(started), "right-click should not open")
}

func TestMouseWheel_ScrollsRowSelectionAndClearsHighlight(t *testing.T) {
	m, _ := zoneTestModel(t)
	_ = m.View()

	m.sel = textSelection{active: true, anchorX: 0, anchorY: 5, cursorX: 9, cursorY: 5}

	updated, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	m = updated.(Model)
	require.Equal(t, 1, m.prs[0].CurrRow(), "wheel down should advance the selection")
	require.False(t, m.sel.active, "scrolling moves rows, so the highlight must drop")

	updated, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	m = updated.(Model)
	require.Equal(t, 0, m.prs[0].CurrRow(), "wheel up should move it back")
}

func TestKeyPress_ClearsHighlight(t *testing.T) {
	m, _ := zoneTestModel(t)
	_ = m.View()
	m.sel = textSelection{active: true, anchorX: 0, anchorY: 5, cursorX: 9, cursorY: 5}

	updated, _ := m.Update(tea.KeyPressMsg{Text: "j"})
	m = updated.(Model)

	require.False(t, m.sel.active, "a keystroke can move what's under the highlight")
}
