package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/common"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/branchsidebar"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/footer"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/issueview"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/notificationview"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prssection"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prview"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/section"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/sidebar"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/tabs"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/keys"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/theme"
)

// bottomPreviewModel builds a PRs view with the preview docked below the list,
// which is the only arrangement with an up/down draggable divider.
func bottomPreviewModel(t *testing.T) Model {
	t.Helper()

	cfg, err := config.ParseConfig(config.Location{
		ConfigFlag:       "../config/testdata/test-config.yml",
		SkipGlobalConfig: true,
	})
	require.NoError(t, err)
	cfg.Defaults.Preview.Open = true
	cfg.Defaults.Preview.Height = 0.4
	cfg.Defaults.Preview.Position = "bottom"

	ctx := &context.ProgramContext{
		Config:       &cfg,
		ScreenWidth:  100,
		ScreenHeight: 40,
		View:         config.PRsView,
		StartTask:    func(task context.Task) tea.Cmd { return nil },
	}
	ctx.Theme = theme.ParseTheme(ctx.Config)
	ctx.Styles = context.InitStyles(ctx.Theme)

	prSection := prssection.NewModel(
		0, ctx,
		config.PrsSectionConfig{Title: "Mine", Filters: "is:open"},
		time.Now(), time.Now(),
	)

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
	m.sidebar.IsOpen = true
	m.sidebar.UpdateProgramContext(ctx)
	m.syncMainContentDimensions()
	m.syncProgramContext()

	return m
}

// ── the arithmetic ───────────────────────────────────────────────────────────

// The divider is the preview's top border, directly under the list.
func TestSeparatorY_IsBelowTheList(t *testing.T) {
	m := bottomPreviewModel(t)

	y, ok := m.separatorY()
	require.True(t, ok)
	require.Equal(t, common.TabsHeight+m.ctx.MainContentHeight, y)
	require.True(t, m.onSeparator(y))
	require.False(t, m.onSeparator(y+1))
}

func TestSeparatorY_NoneWhenPreviewClosedOrDockedRight(t *testing.T) {
	m := bottomPreviewModel(t)

	m.sidebar.IsOpen = false
	_, ok := m.separatorY()
	require.False(t, ok, "no preview, no divider")

	m.sidebar.IsOpen = true
	m.ctx.PreviewPosition = "right"
	_, ok = m.separatorY()
	require.False(t, ok, "a right-docked preview has no up/down divider")
}

// Neither pane may be collapsed to nothing: the divider must stay grabbable.
func TestPreviewHeightForSeparatorY_ClampsBothEnds(t *testing.T) {
	const available = 30

	require.Equal(t, available-1, previewHeightForSeparatorY(-100, available),
		"dragging above the list still leaves one row of list")
	require.Equal(t, 1, previewHeightForSeparatorY(1000, available),
		"dragging past the bottom still leaves one row of preview")

	// Dragging to row TabsHeight+10 leaves a 10-row list.
	require.Equal(t, available-10, previewHeightForSeparatorY(common.TabsHeight+10, available))
}

// ── the wiring ───────────────────────────────────────────────────────────────

// Pressing the divider begins a resize, and must NOT arm a text selection.
func TestSeparator_PressStartsResizeNotSelection(t *testing.T) {
	m := bottomPreviewModel(t)
	y, ok := m.separatorY()
	require.True(t, ok)

	updated, _ := m.Update(tea.MouseClickMsg{X: 10, Y: y, Button: tea.MouseLeft})
	m = updated.(Model)

	require.True(t, m.resizingSeparator, "press on the divider starts a resize")
	require.False(t, m.mouseDown, "it must not arm a drag-selection")
}

func TestSeparator_DragResizesThePreview(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	m := bottomPreviewModel(t)
	before := m.ctx.DynamicPreviewHeight
	y, _ := m.separatorY()

	updated, _ := m.Update(tea.MouseClickMsg{X: 10, Y: y, Button: tea.MouseLeft})
	m = updated.(Model)
	// drag the divider up 5 rows -> the preview grows by 5
	updated, _ = m.Update(tea.MouseMotionMsg{X: 10, Y: y - 5, Button: tea.MouseLeft})
	m = updated.(Model)

	require.Equal(t, before+5, m.ctx.DynamicPreviewHeight, "preview grows as the divider rises")
	require.Equal(t, common.TabsHeight+m.ctx.MainContentHeight, y-5, "divider follows the pointer")
}

// ── persistence ──────────────────────────────────────────────────────────────

func TestSeparator_ReleasePersistsTheHeight(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)

	m := bottomPreviewModel(t)
	m.layoutStateKey = layoutKey("", "")
	y, _ := m.separatorY()

	updated, _ := m.Update(tea.MouseClickMsg{X: 10, Y: y, Button: tea.MouseLeft})
	m = updated.(Model)
	updated, _ = m.Update(tea.MouseMotionMsg{X: 10, Y: y - 4, Button: tea.MouseLeft})
	m = updated.(Model)
	updated, _ = m.Update(tea.MouseReleaseMsg{X: 10, Y: y - 4, Button: tea.MouseLeft})
	m = updated.(Model)

	require.False(t, m.resizingSeparator, "release ends the resize")
	require.Equal(t, m.previewHeightOverride, loadPreviewHeight(m.layoutStateKey),
		"the dragged height should survive into the state file")

	_, err := os.Stat(filepath.Join(dir, "gh-dash", "layout.json"))
	require.NoError(t, err, "state file should exist")
}

func TestLayoutKey_IdentifiesTheDashboard(t *testing.T) {
	t.Setenv("GH_DASH_INSTANCE", "")
	require.Equal(t, "global|", layoutKey("", ""))
	require.Equal(t, "/tmp/c.yml|/repo", layoutKey("/tmp/c.yml", "/repo"))

	t.Setenv("GH_DASH_INSTANCE", "left-window")
	require.Equal(t, "instance:left-window", layoutKey("/tmp/c.yml", "/repo"),
		"GH_DASH_INSTANCE gives a window its own remembered layout")
}

func TestLayoutState_RoundTripsAndMergesKeys(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	require.NoError(t, savePreviewHeight("a", 12))
	require.NoError(t, savePreviewHeight("b", 7))

	require.Equal(t, 12, loadPreviewHeight("a"), "saving b must not clobber a")
	require.Equal(t, 7, loadPreviewHeight("b"))
	require.Equal(t, 0, loadPreviewHeight("never-seen"))
}

func TestLayoutState_NonPositiveHeightIsNotPersisted(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	require.NoError(t, savePreviewHeight("a", 0))
	require.Equal(t, 0, loadPreviewHeight("a"))
}

// A corrupt state file must not take the dashboard down with it.
func TestLayoutState_CorruptFileFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "gh-dash"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "gh-dash", "layout.json"), []byte("{not json"), 0o644))

	require.Equal(t, 0, loadPreviewHeight("a"), "corrupt state reads as 'nothing remembered'")
	require.NoError(t, savePreviewHeight("a", 9), "and can be overwritten")
	require.Equal(t, 9, loadPreviewHeight("a"))
}

// A height remembered from a taller terminal must not swallow the whole list.
func TestSeparator_RememberedHeightIsClampedToTheTerminal(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	m := bottomPreviewModel(t)
	m.previewHeightOverride = 10_000 // as if saved on a much taller screen
	m.syncMainContentDimensions()

	require.GreaterOrEqual(t, m.ctx.MainContentHeight, 1, "the list must keep at least one row")
	require.Less(t, m.ctx.DynamicPreviewHeight, m.getBaseContentHeight())
}

// Sanity: the on-disk shape is the documented one.
func TestLayoutState_FileShape(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	require.NoError(t, savePreviewHeight("global|", 14))

	b, err := os.ReadFile(filepath.Join(dir, "gh-dash", "layout.json"))
	require.NoError(t, err)

	var st layoutState
	require.NoError(t, json.Unmarshal(b, &st))
	require.Equal(t, 14, st.PreviewHeight["global|"])
}
