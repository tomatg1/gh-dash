package tui

import (
	"github.com/dlvhdr/gh-dash/v4/internal/tui/common"
)

// The divider between the list and the bottom preview pane is the preview's
// one-row top border. It sits directly under the list, so its screen row is
// just the tabs plus however tall the list currently is.
//
// separatorY reports that row. ok is false when there is nothing to drag --
// the preview is closed, or it is docked to the right rather than the bottom.
func (m Model) separatorY() (int, bool) {
	if !m.sidebar.IsOpen || m.ctx.PreviewPosition != "bottom" {
		return 0, false
	}

	return common.TabsHeight + m.ctx.MainContentHeight, true
}

// onSeparator reports whether screen row y lands on the draggable divider.
func (m Model) onSeparator(y int) bool {
	sep, ok := m.separatorY()

	return ok && y == sep
}

// maxPreviewHeight caps the preview so the list always keeps at least
// common.MinListHeight rows. The list can't render shorter than that floor, so a
// taller preview doesn't shrink it -- it overflows and pushes the footer off the
// bottom. Falls back to a 1-row preview when the terminal is too short to honor
// the floor at all (an extreme we can't otherwise satisfy).
func maxPreviewHeight(available int) int {
	if hi := available - common.MinListHeight; hi >= 1 {
		return hi
	}

	return 1
}

// previewHeightForSeparatorY converts a divider screen row into a preview
// height, always leaving the list its minimum height and at least one row of
// preview -- a zero-height pane is unrecoverable with the mouse, since its
// divider would then be unreachable.
func previewHeightForSeparatorY(y, available int) int {
	return clampSel(available-(y-common.TabsHeight), 1, maxPreviewHeight(available))
}

// setPreviewHeightFromSeparatorY drags the divider to screen row y.
func (m *Model) setPreviewHeightFromSeparatorY(y int) {
	available := m.getBaseContentHeight() - m.ctx.Styles.Sidebar.BorderWidth
	if available < 2 {
		return
	}

	m.previewHeightOverride = previewHeightForSeparatorY(y, available)

	m.syncMainContentDimensions()
	m.syncProgramContext()
}
