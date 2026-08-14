package data

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMergedSinceQuery(t *testing.T) {
	since := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)

	// is:open is swapped for is:merged; scope tokens are kept; merged:>= is added.
	q := MergedSinceQuery("org:X is:open author:@me created:>=2026-07-13 sort:created-desc", since)
	require.NotContains(t, q, "is:open")
	require.Contains(t, q, "is:merged")
	require.Contains(t, q, "merged:>=2026-07-20T10:00:00Z")
	require.Contains(t, q, "org:X")
	require.Contains(t, q, "author:@me")

	// no is:open -> is:merged is appended.
	q2 := MergedSinceQuery("repo:o/r author:@me", since)
	require.Contains(t, q2, "repo:o/r")
	require.Contains(t, q2, "is:merged")
	require.Contains(t, q2, "merged:>=2026-07-20T10:00:00Z")
}
