package common

import (
	"fmt"

	zone "github.com/lrstanley/bubblezone/v2"
)

// Clickable targets inside a table row. A row-wide zone already exists
// (table.RowZoneID); these are the sub-regions that mean something more
// specific than "this row", so the click handler must test them first.
const (
	ZoneNumber   = "number"   // the "#1234" in the row's title line -> open the PR
	ZoneCi       = "ci"       // the checks icon                     -> open /checks
	ZoneReview   = "review"   // the review icon                     -> open /files
	ZoneComments = "comments" // the comment count                   -> open the conversation
)

// RowTargetZoneID names a clickable region inside row rowIdx. Defined here so
// the renderers that mark these zones and the click handler that reads them
// cannot drift apart.
func RowTargetZoneID(rowIdx int, target string) string {
	return fmt.Sprintf("row-%d-%s", rowIdx, target)
}

// MarkZone wraps bubblezone's Mark, tolerating an uninitialized global manager.
//
// zone.Mark panics when the global manager is nil. Components mark zones while
// rendering, and rendering happens in places the program never runs -- tests,
// headless snapshots -- where nobody called zone.NewGlobal(). Degrade to the
// unmarked string there instead of panicking; the zone simply isn't clickable,
// which is exactly right when there's no mouse to click it.
func MarkZone(id, v string) string {
	if zone.DefaultManager == nil {
		return v
	}

	return zone.Mark(id, v)
}
