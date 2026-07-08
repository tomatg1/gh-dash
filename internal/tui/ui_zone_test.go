package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/data"
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

// zoneTestModel builds a PRs view with two rows and one tab, rendered wide
// enough that nothing is truncated away.
func zoneTestModel(t *testing.T) Model {
	t.Helper()

	zone.NewGlobal()
	zone.SetEnabled(true)
	t.Cleanup(func() { zone.SetEnabled(false) })

	cfg, err := config.ParseConfig(config.Location{
		ConfigFlag:       "../config/testdata/test-config.yml",
		SkipGlobalConfig: true,
	})
	require.NoError(t, err)

	ctx := &context.ProgramContext{
		Config:       &cfg,
		ScreenWidth:  160,
		ScreenHeight: 40,
		View:         config.PRsView,
		StartTask:    func(task context.Task) tea.Cmd { return nil },
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
		{Primary: &data.PullRequestData{Number: 1, Title: "first pr", State: "OPEN"}},
		{Primary: &data.PullRequestData{Number: 2, Title: "second pr", State: "OPEN"}},
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
	}
	m.tabs.SetSections(m.prs)
	m.syncMainContentDimensions()
	m.syncProgramContext()

	return m
}

// The tab title is marked before it enters the carousel, which runs
// lipgloss.Width and ansi.Truncate over it. Those must leave bubblezone's
// private CSI markers intact or the tab is silently unclickable.
func TestMouseZones_TabSurvivesCarouselTruncation(t *testing.T) {
	m := zoneTestModel(t)

	_ = m.View() // View() runs zone.Scan over the composed frame

	tab := waitForZone(t, tabs.TabZoneID(0))
	require.False(t, tab.IsZero(), "tab zone should register")
}

// Rows are marked outside the row style's MaxWidth, so their markers must not
// be sliced. Row 0 must also render above row 1.
func TestMouseZones_RowsRegisterInOrder(t *testing.T) {
	m := zoneTestModel(t)

	_ = m.View()

	row0 := waitForZone(t, table.RowZoneID(0))
	row1 := waitForZone(t, table.RowZoneID(1))

	require.Less(t, row0.StartY, row1.StartY, "row 0 should render above row 1")
}

func TestMouseClick_OnRowSelectsIt(t *testing.T) {
	m := zoneTestModel(t)

	_ = m.View()
	row1 := waitForZone(t, table.RowZoneID(1))

	require.Equal(t, 0, m.prs[0].CurrRow(), "row 0 selected before the click")

	updated, _ := m.Update(tea.MouseClickMsg{
		X:      row1.StartX + 1,
		Y:      row1.StartY,
		Button: tea.MouseLeft,
	})
	m = updated.(Model)

	require.Equal(t, 1, m.prs[0].CurrRow(), "clicking row 1 should select row 1")
}

// A click on empty space must not move the selection or open anything.
func TestMouseClick_OutsideAnyZoneIsInert(t *testing.T) {
	m := zoneTestModel(t)

	_ = m.View()
	waitForZone(t, table.RowZoneID(0))

	updated, cmd := m.Update(tea.MouseClickMsg{X: 0, Y: 39, Button: tea.MouseLeft})
	m = updated.(Model)

	require.Equal(t, 0, m.prs[0].CurrRow(), "selection should not move")
	require.Nil(t, cmd, "no command should be issued for a click on nothing")
}

// Right-clicks are ignored outright.
func TestMouseClick_NonLeftButtonIgnored(t *testing.T) {
	m := zoneTestModel(t)

	_ = m.View()
	row1 := waitForZone(t, table.RowZoneID(1))

	updated, _ := m.Update(tea.MouseClickMsg{
		X:      row1.StartX + 1,
		Y:      row1.StartY,
		Button: tea.MouseRight,
	})
	m = updated.(Model)

	require.Equal(t, 0, m.prs[0].CurrRow(), "right-click should not select")
}

func TestMouseWheel_ScrollsRowSelection(t *testing.T) {
	m := zoneTestModel(t)
	_ = m.View()

	updated, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	m = updated.(Model)
	require.Equal(t, 1, m.prs[0].CurrRow(), "wheel down should advance the selection")

	updated, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	m = updated.(Model)
	require.Equal(t, 0, m.prs[0].CurrRow(), "wheel up should move it back")
}
