package data

import (
	"fmt"
	"sort"
	"strings"
	"time"

	graphql "github.com/cli/shurcooL-graphql"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
)

// FetchMergeQueueNumbers returns the set of PR numbers currently sitting in a
// repo's merge queue for the given base branch.
//
// This is the authoritative source, and the list is enriched from it because the
// search fetch was observed reading isInMergeQueue as false on queued PRs. Note
// that search has since been measured returning the field correctly (repo- and
// org-scoped, zero misses against this query), so the original explanation --
// "search doesn't populate the expensive computed field" -- does not hold; the
// unfalsified one is that search lags for the first seconds after an enqueue,
// which is exactly when the icon is looked for. tooling/FORK_NOTES.md has the
// measurements.
func FetchMergeQueueNumbers(owner, name, branch string) (map[int]bool, error) {
	if client == nil {
		return nil, nil
	}

	var q struct {
		Repository struct {
			MergeQueue struct {
				Entries struct {
					Nodes []struct {
						PullRequest struct{ Number int }
					}
				} `graphql:"entries(first: 100)"`
			} `graphql:"mergeQueue(branch: $branch)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}
	vars := map[string]any{
		"owner":  graphql.String(owner),
		"name":   graphql.String(name),
		"branch": graphql.String(branch),
	}
	if err := client.Query("MergeQueue", &q, vars); err != nil {
		return nil, err
	}

	out := make(map[int]bool)
	for _, n := range q.Repository.MergeQueue.Entries.Nodes {
		out[n.PullRequest.Number] = true
	}

	return out, nil
}

// EnrichMergeQueueStatus sets IsInMergeQueue on the PRs whose repo is configured
// as a merge-queue repo (defaults.mergeQueueRepos) and which are actually in the
// queue. Best-effort: a failed query for one repo just leaves those PRs
// un-flagged. Scoped to mergeQueueRepos so it costs one query per distinct
// (repo, base branch) present, not one per repo in the results.
func EnrichMergeQueueStatus(d config.Defaults, prs []PullRequestData) {
	if len(prs) == 0 {
		return
	}

	// queued[repo][number] = true, filled once per distinct (repo, base).
	queued := make(map[string]map[int]bool)
	fetched := make(map[string]bool) // repo\x00base already queried

	for i := range prs {
		repo := prs[i].GetRepoNameWithOwner()
		if !d.UsesMergeQueue(repo) {
			continue
		}
		base := prs[i].BaseRefName
		key := repo + "\x00" + base
		if fetched[key] {
			continue
		}
		fetched[key] = true

		owner, name := prs[i].GetRepoNameAndOwner()
		nums, err := FetchMergeQueueNumbers(owner, name, base)
		if err != nil {
			continue
		}
		if queued[repo] == nil {
			queued[repo] = make(map[int]bool)
		}
		for n := range nums {
			queued[repo][n] = true
		}
	}

	for i := range prs {
		if q := queued[prs[i].GetRepoNameWithOwner()]; q != nil && q[prs[i].Number] {
			prs[i].IsInMergeQueue = true
		}
	}
}

// SortByMergedAtDesc orders PRs most-recently-merged first.
//
// Search returns results in the filter's own sort order (typically
// `sort:created-desc`), which is NOT merge order — a PR opened days ago and merged
// a minute ago sorts below one opened today and merged yesterday. Anything without
// a merge timestamp sorts last, so a stray non-merged row can't head the list.
func SortByMergedAtDesc(prs []PullRequestData) {
	sort.SliceStable(prs, func(i, j int) bool {
		a, b := prs[i].MergedAt, prs[j].MergedAt
		if a == nil || b == nil {
			return a != nil // non-nil before nil; equal-nil keeps existing order
		}

		return a.After(*b)
	})
}

// MergedSinceQuery derives a "recently merged" search filter from a section's
// filter: swap is:open for is:merged (or append is:merged) and constrain to PRs
// merged at or after `since`. Everything else in the filter (org:/repo:/author:/
// labels) is kept, so it mirrors the section's scope.
func MergedSinceQuery(baseFilter string, since time.Time) string {
	f := baseFilter
	if strings.Contains(f, "is:open") {
		f = strings.Replace(f, "is:open", "is:merged", 1)
	} else {
		f = strings.TrimSpace(f + " is:merged")
	}

	return fmt.Sprintf("%s merged:>=%s", f, since.UTC().Format(time.RFC3339))
}
