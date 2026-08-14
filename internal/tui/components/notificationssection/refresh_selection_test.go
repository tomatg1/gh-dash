package notificationssection

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/data"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/notificationrow"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/section"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/theme"
)

func refreshTestCtx(t *testing.T) *context.ProgramContext {
	t.Helper()
	cfg, err := config.ParseConfig(config.Location{
		ConfigFlag:       "../../../config/testdata/test-config.yml",
		SkipGlobalConfig: true,
	})
	require.NoError(t, err)
	// The test config has no notifications sections; add one.
	cfg.NotificationsSections = []config.NotificationsSectionConfig{{Title: "Inbox"}}
	ctx := &context.ProgramContext{
		Config:            &cfg,
		ScreenWidth:       100,
		ScreenHeight:      40,
		MainContentWidth:  100,
		MainContentHeight: 20,
		View:              config.NotificationsView,
		StartTask:         func(task context.Task) tea.Cmd { return nil },
	}
	ctx.Theme = theme.ParseTheme(ctx.Config)
	ctx.Styles = context.InitStyles(ctx.Theme)
	return ctx
}

// A CheckSuite notification returns ResolvedUrl from GetUrl, giving each row a
// distinct URL without needing a full repo/subject fixture.
func notif(id, url string) notificationrow.Data {
	return notificationrow.Data{
		Notification: data.NotificationData{
			Id:      id,
			Subject: data.NotificationSubject{Type: "CheckSuite"},
		},
		ResolvedUrl: url,
	}
}

// Regression: a refresh must not flash the notification cursor to the top before
// the fetch lands. FetchAllSections must carry the rows AND cursor over now.
func TestRefresh_NotificationSelectionDoesNotFlashToTop(t *testing.T) {
	ctx := refreshTestCtx(t)

	old := NewModel(1, ctx, ctx.Config.NotificationsSections[0], time.Now())
	old.Notifications = []notificationrow.Data{
		notif("n1", "https://github.com/o/r/n/1"),
		notif("n2", "https://github.com/o/r/n/2"),
		notif("n3", "https://github.com/o/r/n/3"),
		notif("n4", "https://github.com/o/r/n/4"),
	}
	old.Table.SetRows(old.BuildRows())
	old.Table.SetCurrItem(2)
	wantURL := old.GetCurrRow().GetUrl()
	require.Equal(t, "https://github.com/o/r/n/3", wantURL, "precondition: 3rd notification selected")

	oldNotifs := make([]section.Section, len(ctx.Config.NotificationsSections)+1)
	oldNotifs[1] = &old
	newSections, _ := FetchAllSections(ctx, oldNotifs)

	ns := newSections[0].(*Model)
	ns.Table.SetRows(ns.BuildRows()) // render loop rebuilds rows before the fetch lands

	require.Equal(t, 4, len(ns.Notifications), "carried-over rows present immediately")
	require.Equal(t, 2, ns.Table.GetCurrItem(),
		"cursor must carry over on refresh, not flash to the top row")
	require.NotNil(t, ns.GetCurrRow())
	require.Equal(t, wantURL, ns.GetCurrRow().GetUrl(),
		"the same notification must stay selected across the refresh")
}
