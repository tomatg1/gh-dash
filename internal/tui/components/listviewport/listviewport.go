package listviewport

import (
	"time"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"

	"github.com/dlvhdr/gh-dash/v4/internal/tui/constants"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
	"github.com/dlvhdr/gh-dash/v4/internal/utils"
)

type Model struct {
	ctx             context.ProgramContext
	viewport        viewport.Model
	topBoundId      int
	bottomBoundId   int
	currId          int
	ListItemHeight  int
	NumCurrentItems int
	NumTotalItems   int
	LastUpdated     time.Time
	CreatedAt       time.Time
	ItemTypeLabel   string

	// View is called on every frame and its output only changes with the
	// content, the scroll offset or the size, so the last render is kept. It sits
	// behind a pointer because callers render through value copies.
	contentGen int
	memo       *viewMemo
}

type viewKey struct{ gen, yOffset, width, height int }

type viewMemo struct {
	key viewKey
	out string
	ok  bool
}

func NewModel(
	ctx context.ProgramContext,
	dimensions constants.Dimensions,
	lastUpdated time.Time,
	createdAt time.Time,
	itemTypeLabel string,
	numItems, listItemHeight int,
) Model {
	model := Model{
		ctx:             ctx,
		NumCurrentItems: numItems,
		ListItemHeight:  listItemHeight,
		currId:          0,
		viewport: viewport.New(
			viewport.WithWidth(dimensions.Width),
			viewport.WithHeight(dimensions.Height),
		),
		topBoundId:    0,
		memo:          &viewMemo{},
		ItemTypeLabel: itemTypeLabel,
		LastUpdated:   lastUpdated,
		CreatedAt:     createdAt,
	}
	model.fitWindow()
	return model
}

// The visible window is defined by topBoundId alone. bottomBoundId and the
// viewport's scroll offset are DERIVED from it, in fitWindow and nowhere else.
// They used to be updated independently by each method -- a rebuild reset one,
// a resize touched none, a jump to the end moved only the offset -- and every
// disagreement between them showed up as the list moving under the selection.

// fitWindow clamps the window to the list, keeps the cursor inside it while
// moving the window as little as possible, and derives the bottom bound and the
// scroll offset from the top.
func (m *Model) fitWindow() {
	n := m.NumCurrentItems
	perPage := m.getNumPrsPerPage()
	if n <= 0 || perPage <= 0 {
		// Nothing to show, or no height yet (before the first size message):
		// keep the top where it is so a later resize can fit it, and leave the
		// cursor alone -- clamping it into an empty window would reset it.
		m.bottomBoundId = utils.Min(n-1, m.topBoundId+perPage-1)
		return
	}

	top := m.topBoundId
	if m.currId < top {
		top = m.currId
	}
	if m.currId > top+perPage-1 {
		top = m.currId - perPage + 1
	}
	top = utils.Max(0, utils.Min(top, n-perPage))

	m.topBoundId = top
	m.bottomBoundId = utils.Min(n-1, top+perPage-1)
	m.viewport.SetYOffset(top * m.ListItemHeight)
}

// SetNumItems updates the item count, keeping the window where the scroll left
// it (a rebuild with the same rows must not move anything) and only pulling it,
// and the cursor, back inside a list that shrank.
func (m *Model) SetNumItems(numItems int) {
	m.NumCurrentItems = numItems
	if numItems > 0 {
		m.currId = utils.Min(m.currId, numItems-1)
	}
	m.fitWindow()
}

func (m *Model) SetTotalItems(total int) {
	m.NumTotalItems = total
}

func (m *Model) SetItemHeight(height int) {
	m.ListItemHeight = height
	m.fitWindow()
}

// SyncViewPort replaces the rendered content. The scroll offset is re-applied
// afterwards because the viewport clamps it to the content it has: an offset set
// while the content was still empty (a freshly built list) would be lost.
func (m *Model) SyncViewPort(content string) {
	m.contentGen++
	m.viewport.SetContent(content)
	m.viewport.SetYOffset(m.topBoundId * m.ListItemHeight)
}

func (m *Model) getNumPrsPerPage() int {
	if m.ListItemHeight == 0 {
		return 0
	}
	return m.viewport.Height() / m.ListItemHeight
}

func (m *Model) ResetCurrItem() {
	m.currId = 0
	m.topBoundId = 0
	m.fitWindow()
}

func (m *Model) GetCurrItem() int {
	return m.currId
}

func (m *Model) NextItem() int {
	m.currId = utils.Max(0, utils.Min(m.currId+1, m.NumCurrentItems-1))
	m.fitWindow()
	return m.currId
}

func (m *Model) PrevItem() int {
	m.currId = utils.Max(m.currId-1, 0)
	m.fitWindow()
	return m.currId
}

func (m *Model) FirstItem() int {
	m.currId = 0
	m.fitWindow()
	return m.currId
}

func (m *Model) LastItem() int {
	m.currId = utils.Max(0, m.NumCurrentItems-1)
	m.fitWindow()
	return m.currId
}

// SetCurrItem jumps the cursor to an absolute item index (a click, a restored
// selection), moving the window only as far as needed to keep it visible.
func (m *Model) SetCurrItem(id int) int {
	if m.NumCurrentItems == 0 {
		return m.currId
	}
	m.currId = utils.Max(0, utils.Min(id, m.NumCurrentItems-1))
	m.fitWindow()
	return m.currId
}

// ScrollState is where this list is scrolled to: the first visible item and the
// cursor. A rebuilt list restores it with RestoreScroll so the selected row stays
// on the same screen line -- restoring the cursor alone leaves it wherever a
// fresh scroll from the top happens to put it.
func (m *Model) ScrollState() (top, curr int) {
	return m.topBoundId, m.currId
}

// RestoreScroll puts back a ScrollState taken from a previous instance of this
// list, clamped to the current one.
func (m *Model) RestoreScroll(top, curr int) {
	if m.NumCurrentItems > 0 {
		curr = utils.Max(0, utils.Min(curr, m.NumCurrentItems-1))
	}
	m.currId = curr
	m.topBoundId = utils.Max(0, top)
	m.fitWindow()
}

func (m *Model) SetDimensions(dimensions constants.Dimensions) {
	m.viewport.SetHeight(max(0, dimensions.Height))
	m.viewport.SetWidth(max(0, dimensions.Width))
	m.fitWindow() // a different height means a different window
}

func (m *Model) View() string {
	key := viewKey{m.contentGen, m.viewport.YOffset(), m.viewport.Width(), m.viewport.Height()}
	if m.memo != nil && m.memo.ok && m.memo.key == key {
		return m.memo.out
	}

	out := lipgloss.NewStyle().
		Width(m.viewport.Width()).
		MaxWidth(m.viewport.Width()).
		Render(m.viewport.View())
	if m.memo != nil {
		*m.memo = viewMemo{key: key, out: out, ok: true}
	}

	return out
}

func (m *Model) UpdateProgramContext(ctx *context.ProgramContext) {
	m.ctx = *ctx
}
