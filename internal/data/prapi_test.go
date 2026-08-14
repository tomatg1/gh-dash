package data

import (
	"testing"

	gh "github.com/cli/go-gh/v2/pkg/api"
	"github.com/stretchr/testify/require"
)

func TestClearEnrichmentCache(t *testing.T) {
	// Save original state
	originalCachedClient := cachedClient
	defer func() {
		cachedClient = originalCachedClient
	}()

	t.Run("clears nil cache without panic", func(t *testing.T) {
		cachedClient = nil
		require.True(t, IsEnrichmentCacheCleared(), "cache should be cleared initially")

		ClearEnrichmentCache()
		require.True(t, IsEnrichmentCacheCleared(), "cache should remain cleared")
	})

	t.Run("clears non-nil cache", func(t *testing.T) {
		// Simulate having a cached client (we use an empty struct pointer
		// since we can't create a real GraphQL client without credentials)
		cachedClient = &gh.GraphQLClient{}
		require.False(
			t,
			IsEnrichmentCacheCleared(),
			"cache should not be cleared when client is set",
		)

		ClearEnrichmentCache()
		require.True(
			t,
			IsEnrichmentCacheCleared(),
			"cache should be cleared after ClearEnrichmentCache",
		)
	})
}

func TestIsEnrichmentCacheCleared(t *testing.T) {
	// Save original state
	originalCachedClient := cachedClient
	defer func() {
		cachedClient = originalCachedClient
	}()

	t.Run("returns true when cache is nil", func(t *testing.T) {
		cachedClient = nil
		require.True(t, IsEnrichmentCacheCleared())
	})

	t.Run("returns false when cache is set", func(t *testing.T) {
		cachedClient = &gh.GraphQLClient{}
		require.False(t, IsEnrichmentCacheCleared())
	})
}

func TestSetClient(t *testing.T) {
	// Save original state
	originalClient := client
	originalCachedClient := cachedClient
	defer func() {
		client = originalClient
		cachedClient = originalCachedClient
	}()

	t.Run("sets both client and cachedClient", func(t *testing.T) {
		client = nil
		cachedClient = nil

		// SetClient with nil should set both to nil
		SetClient(nil)
		require.Nil(t, client)
		require.True(t, IsEnrichmentCacheCleared())
	})
}

// A PR previewed from a notification is fetched whole (resource(url:), which
// unlike search computes isInMergeQueue) and then converted for display. The
// merge-queue actions run off that converted copy: enqueue/dequeue address the PR
// by node id, and which of the two `m` means depends on the queue state. Drop
// either in the conversion and the notifications preview mutates nothing (empty
// id) or can only ever add to the queue.
func TestToPullRequestData_KeepsWhatTheMergeQueueActionsNeed(t *testing.T) {
	enriched := EnrichedPullRequestData{
		Number:         7970,
		Id:             "PR_kwDOO-ojmc7-0y3Q",
		IsInMergeQueue: true,
	}

	pr := enriched.ToPullRequestData()

	require.Equal(t, "PR_kwDOO-ojmc7-0y3Q", pr.Id, "enqueue/dequeue address the PR by node id")
	require.True(t, pr.IsInMergeQueue, "queue state decides whether `m` enqueues or dequeues")
}
