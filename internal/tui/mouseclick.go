package tui

import (
	"strings"
	"time"
)

// nowFunc is a seam for tests, which cannot wait out a real double-click window.
var nowFunc = time.Now

// doubleClickWindow is how long a second click has to arrive to count as a
// double-click. Bubbletea reports presses and releases but no click count, so
// the pairing is measured here.
const doubleClickWindow = 400 * time.Millisecond

// registerRowClick records a bare click on a row and reports whether it
// completes a double-click on that same row.
//
// A completed pair is consumed, so a third click starts a fresh one rather than
// opening the PR again.
func (m *Model) registerRowClick(row int) bool {
	now := nowFunc()

	if m.lastClickRow == row && now.Sub(m.lastClickAt) <= doubleClickWindow {
		m.lastClickRow, m.lastClickAt = -1, time.Time{}
		return true
	}

	m.lastClickRow, m.lastClickAt = row, now

	return false
}

// subPageURL turns a PR URL into one of its tabs, e.g. ".../pull/12/checks".
func subPageURL(prURL, page string) string {
	if prURL == "" {
		return ""
	}

	return strings.TrimRight(prURL, "/") + "/" + page
}

// rowHit says which clickable region of a row the pointer landed on. A row-wide
// hit is implied; these are the sub-regions that mean something more specific.
type rowHit struct {
	number   bool
	ci       bool
	review   bool
	comments bool
}

// targeted reports whether the click landed on a sub-region rather than bare row.
func (h rowHit) targeted() bool {
	return h.number || h.ci || h.review || h.comments
}

// rowClickAction decides which URL a click on a row should open, or "" when the
// click should merely select the row.
//
// Kept pure so the mapping from gesture to URL is testable without executing a
// tea.Cmd (which would launch a real browser).
func rowClickAction(url string, hit rowHit, isDoubleClick bool) string {
	switch {
	case hit.number:
		return url
	case hit.ci:
		return subPageURL(url, "checks")
	case hit.review:
		return subPageURL(url, "files")
	case hit.comments:
		return url
	case isDoubleClick:
		return url
	}

	return ""
}
