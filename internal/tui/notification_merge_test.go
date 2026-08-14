package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/data"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/footer"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/notificationview"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prrow"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prview"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/keys"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/theme"
)

// notificationPRModel builds a notifications-view model previewing pr, with the
// tasks it fires captured rather than run — every task constructor calls
// ctx.StartTask synchronously, before the `gh` process is built, so this sees
// which task was chosen without shelling out.
func notificationPRModel(
	t *testing.T,
	pr *prrow.Data,
	tune func(*config.Defaults),
) (Model, *[]context.Task) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir()) // no remembered merge method leaks in

	cfg, err := config.ParseConfig(config.Location{
		ConfigFlag:       "../config/testdata/test-config.yml",
		SkipGlobalConfig: true,
	})
	require.NoError(t, err)
	tune(&cfg.Defaults)

	var started []context.Task
	ctx := &context.ProgramContext{
		Config: &cfg,
		View:   config.NotificationsView,
		StartTask: func(task context.Task) tea.Cmd {
			started = append(started, task)
			return nil
		},
	}
	ctx.Theme = theme.ParseTheme(ctx.Config)
	ctx.Styles = context.InitStyles(ctx.Theme)

	m := Model{
		ctx:              ctx,
		keys:             keys.Keys,
		footer:           footer.NewModel(ctx),
		prView:           prview.NewModel(ctx),
		notificationView: notificationview.NewModel(ctx),
	}
	m.notificationView.SetSubjectPR(pr, "notif-1")

	return m, &started
}

func queuedPR(number int, inQueue bool) *prrow.Data {
	return &prrow.Data{Primary: &data.PullRequestData{
		Number:         number,
		Id:             "PR_kwnodeid",
		IsInMergeQueue: inQueue,
		Repository:     data.Repository{NameWithOwner: "owner/repo"},
	}}
}

// Merging a PR from a notification preview must resolve exactly as it does from
// the PRs list: whether a repo uses a merge queue, and which strategy it merges
// with, are facts about the repo, not about the pane you pressed `m` in.
func TestNotificationPRMerge_ResolvesLikeThePRsView(t *testing.T) {
	tests := []struct {
		name          string
		inQueue       bool
		defaults      func(*config.Defaults)
		wantTaskId    string
		wantStartText string
	}{
		{
			name:          "merge-queue repo, not queued: adds it to the queue",
			defaults:      func(d *config.Defaults) { d.MergeQueueRepos = []string{"owner/repo"} },
			wantTaskId:    "pr_enqueue_123",
			wantStartText: "Adding PR #123 to the merge queue",
		},
		{
			name:          "merge-queue repo, already queued: takes it back out",
			inQueue:       true,
			defaults:      func(d *config.Defaults) { d.MergeQueueRepos = []string{"owner/repo"} },
			wantTaskId:    "pr_dequeue_123",
			wantStartText: "Removing PR #123 from the merge queue",
		},
		{
			name:          "configured strategy: merges with it, so gh never prompts",
			defaults:      func(d *config.Defaults) { d.MergeMethod = "squash" },
			wantTaskId:    "pr_merge_123",
			wantStartText: "Merging PR #123 (squash)",
		},
		{
			name:          "no strategy configured or remembered: plain interactive merge",
			defaults:      func(d *config.Defaults) {},
			wantTaskId:    "merge_123",
			wantStartText: "Merging PR #123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, started := notificationPRModel(t, queuedPR(123, tt.inQueue), tt.defaults)

			next, _ := m.Update(tea.KeyPressMsg{Text: "m"})
			m = next.(Model)
			require.True(t, m.notificationView.HasPendingAction(), "`m` should open a confirmation")

			next, cmd := m.Update(tea.KeyPressMsg{Text: "y"})
			m = next.(Model)
			require.NotNil(t, cmd, "confirming should return a command")

			require.Len(t, *started, 1, "confirming should start exactly one task")
			require.Equal(t, tt.wantTaskId, (*started)[0].Id)
			require.Equal(t, tt.wantStartText, (*started)[0].StartText)
		})
	}
}
