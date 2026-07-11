package tui

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/data"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prrow"
)

// mergeAction decides what the `m` key does. On a merge-queue repo it toggles
// the queue; elsewhere it's a plain merge.
func TestMergeAction(t *testing.T) {
	m := bottomPreviewModel(t)

	row := &prrow.Data{Primary: &data.PullRequestData{
		Repository: data.Repository{NameWithOwner: "owner/repo"},
	}}

	// Not a merge-queue repo -> plain merge, regardless of queue state.
	m.ctx.Config.Defaults.MergeQueueRepos = nil
	require.Equal(t, "merge", m.mergeAction(row))
	row.Primary.IsInMergeQueue = true
	require.Equal(t, "merge", m.mergeAction(row))

	// Merge-queue repo -> toggle on the queue state.
	m.ctx.Config.Defaults.MergeQueueRepos = []string{"owner/repo"}
	row.Primary.IsInMergeQueue = false
	require.Equal(t, "enqueue", m.mergeAction(row), "out of the queue -> enqueue")
	row.Primary.IsInMergeQueue = true
	require.Equal(t, "dequeue", m.mergeAction(row), "already queued -> dequeue")

	// A different repo isn't affected.
	row.Primary.Repository.NameWithOwner = "owner/other"
	row.Primary.IsInMergeQueue = false
	require.Equal(t, "merge", m.mergeAction(row))

	// A nil / non-PR row is always a plain merge (never panics).
	require.Equal(t, "merge", m.mergeAction(nil))
}
