package tui

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/data"
	"github.com/dlvhdr/gh-dash/v4/internal/prefs"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prrow"
)

// mergeAction decides what the `m` key does: toggle the merge queue on a
// merge-queue repo, otherwise merge with an explicit strategy — asking for one
// only when neither the config nor the remembered choice supplies it.
func TestMergeAction(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir()) // isolate from the real prefs file
	m := bottomPreviewModel(t)

	row := &prrow.Data{Primary: &data.PullRequestData{
		Repository: data.Repository{NameWithOwner: "owner/repo"},
	}}

	// No queue, nothing configured or remembered -> ask for the method once.
	m.ctx.Config.Defaults.MergeQueueRepos = nil
	require.Equal(t, "merge_method", m.mergeAction(row))
	row.Primary.IsInMergeQueue = true
	require.Equal(t, "merge_method", m.mergeAction(row), "queue state is irrelevant off-queue")
	row.Primary.IsInMergeQueue = false

	// Configured globally -> merge straight away with that strategy, no prompt
	// for the method.
	m.ctx.Config.Defaults.MergeMethod = "squash"
	require.Equal(t, "merge_squash", m.mergeAction(row))

	// A per-repo entry beats the global default.
	m.ctx.Config.Defaults.MergeMethodRepos = map[string]string{"owner/repo": "rebase"}
	require.Equal(t, "merge_rebase", m.mergeAction(row))
	m.ctx.Config.Defaults.MergeMethod = ""
	m.ctx.Config.Defaults.MergeMethodRepos = nil

	// With nothing configured, a previously remembered answer is used — and it's
	// per repo, so another repo still asks.
	require.NoError(t, prefs.SaveMergeMethod("owner/repo", "squash"))
	require.Equal(t, "merge_squash", m.mergeAction(row), "remembered choice avoids re-asking")
	row.Primary.Repository.NameWithOwner = "owner/unseen"
	require.Equal(t, "merge_method", m.mergeAction(row), "a different repo is asked separately")
	row.Primary.Repository.NameWithOwner = "owner/repo"

	// Merge-queue repo -> toggle on the queue state, never a merge strategy.
	m.ctx.Config.Defaults.MergeQueueRepos = []string{"owner/repo"}
	row.Primary.IsInMergeQueue = false
	require.Equal(t, "enqueue", m.mergeAction(row), "out of the queue -> enqueue")
	row.Primary.IsInMergeQueue = true
	require.Equal(t, "dequeue", m.mergeAction(row), "already queued -> dequeue")

	// A nil / non-PR row falls back to the interactive merge (never panics).
	require.Equal(t, "merge", m.mergeAction(nil))
}
