package data

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The recently-merged search keeps the section's scope but asks for merged PRs
// newest-updated first, with no merged:>= date qualifier: during GitHub's
// 2026-10-22 search incident that qualifier returned nothing while plain
// is:merged search still worked. The tab's own sort token is dropped so the
// sort:updated gh-dash appends takes effect -- sorted by creation, a PR opened
// weeks ago and merged today could fall outside the result limit.
func TestMergedQuery(t *testing.T) {
	require.Equal(t, "org:X is:merged author:@me created:>=2026-07-13",
		MergedQuery("org:X is:open author:@me created:>=2026-07-13 sort:created-desc"),
		"is:open becomes is:merged, the sort token goes, the scope stays")

	require.Equal(t, "repo:o/r author:@me is:merged",
		MergedQuery("repo:o/r author:@me"),
		"without is:open, is:merged is appended")

	require.NotContains(t, MergedQuery("repo:o/r is:open sort:updated-asc"), "merged:",
		"no date qualifier: the window is applied to mergedAt instead")
}

// MergedSince applies the window that the search no longer does.
func TestMergedSince(t *testing.T) {
	since := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	at := func(h int) *time.Time { v := since.Add(time.Duration(h) * time.Hour); return &v }
	prs := []PullRequestData{
		{Number: 1, MergedAt: at(3)},  // inside
		{Number: 2, MergedAt: at(-1)}, // before the window
		{Number: 3, MergedAt: nil},    // not merged
		{Number: 4, MergedAt: at(0)},  // exactly at the boundary: inside
		{Number: 5, MergedAt: at(20)}, // inside
	}

	var got []int
	for _, pr := range MergedSince(prs, since) {
		got = append(got, pr.Number)
	}
	require.Equal(t, []int{1, 4, 5}, got, "only PRs merged at or after the start of the window, order kept")
	require.Empty(t, MergedSince(nil, since))
}
