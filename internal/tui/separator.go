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

// previewHeightForSeparatorY converts a divider screen row into a preview
// height, always leaving at least one row of list and one row of preview -- a
// zero-height pane is unrecoverable with the mouse, since its divider would
// then be unreachable.
func previewHeightForSeparatorY(y, available int) int {
	listHeight := clampSel(y-common.TabsHeight, 1, available-1)

	return available - listHeight
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
