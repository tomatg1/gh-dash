package common

import (
	zone "github.com/lrstanley/bubblezone/v2"
)

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
