package data

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func at(s string) *time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}

	return &t
}

// Recently-merged PRs arrive in the section filter's own sort order (typically
// created-desc), which is NOT merge order: a PR opened days ago and merged a
// minute ago comes back below one opened today and merged yesterday. They must be
// reordered newest-merge-first before display.
func TestSortByMergedAtDesc(t *testing.T) {
	prs := []PullRequestData{
		{Number: 1, MergedAt: at("2026-08-02T18:06:39Z")},
		{Number: 2, MergedAt: at("2026-08-03T10:18:50Z")}, // newest merge, oldest position
		{Number: 3, MergedAt: at("2026-08-02T19:19:47Z")},
		{Number: 4, MergedAt: at("2026-08-03T02:35:21Z")},
	}

	SortByMergedAtDesc(prs)

	got := []int{prs[0].Number, prs[1].Number, prs[2].Number, prs[3].Number}
	require.Equal(t, []int{2, 4, 3, 1}, got, "newest merge first")
}

// An unmerged row (nil MergedAt) must never be promoted above merged ones just
// because it has no timestamp to compare.
func TestSortByMergedAtDesc_NilsSortLast(t *testing.T) {
	prs := []PullRequestData{
		{Number: 1, MergedAt: nil},
		{Number: 2, MergedAt: at("2026-08-01T00:00:00Z")},
		{Number: 3, MergedAt: nil},
		{Number: 4, MergedAt: at("2026-08-03T00:00:00Z")},
	}

	SortByMergedAtDesc(prs)

	require.Equal(t, 4, prs[0].Number)
	require.Equal(t, 2, prs[1].Number)
	require.Equal(t, []int{1, 3}, []int{prs[2].Number, prs[3].Number},
		"nil timestamps keep their relative order at the end")
}

func TestSortByMergedAtDesc_EmptyAndSingle(t *testing.T) {
	require.NotPanics(t, func() { SortByMergedAtDesc(nil) })
	one := []PullRequestData{{Number: 9, MergedAt: at("2026-08-01T00:00:00Z")}}
	SortByMergedAtDesc(one)
	require.Equal(t, 9, one[0].Number)
}
