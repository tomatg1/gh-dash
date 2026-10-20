package listviewport

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/tui/constants"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
)

type testModelOpts struct {
	numItems       int
	viewportHeight int
	itemHeight     int
}

// numItems, viewportHeight, itemHeight int
func newTestModel(opts testModelOpts) Model {
	ctx := context.ProgramContext{}
	dims := constants.Dimensions{Width: 80, Height: opts.viewportHeight}
	now := time.Now()
	return NewModel(ctx, dims, now, now, "items", opts.numItems, opts.itemHeight)
}

func TestPrevItemScrollsAtTopBound(t *testing.T) {
	// Initial bounds: topBoundId=0, bottomBoundId=4, currId=0
	m := newTestModel(testModelOpts{numItems: 10, viewportHeight: 5, itemHeight: 1})

	// Navigate down past the first page boundary
	// After 5 NextItem calls: currId=5, topBoundId=1, bottomBoundId=5
	for range 5 {
		m.NextItem()
	}

	if m.GetCurrItem() != 5 {
		t.Fatalf("expected currId=5 after 5 NextItem calls, got %d", m.GetCurrItem())
	}
	if m.topBoundId != 1 {
		t.Fatalf("expected topBoundId=1, got %d", m.topBoundId)
	}

	// Navigate back up to the top bound
	// 4 PrevItem calls: currId goes 4, 3, 2, 1
	for range 4 {
		m.PrevItem()
	}

	if m.GetCurrItem() != 1 {
		t.Fatalf("expected currId=1, got %d", m.GetCurrItem())
	}
	if m.topBoundId != 1 {
		t.Fatalf("expected topBoundId=1 (no scroll yet), got %d", m.topBoundId)
	}

	// Now one more PrevItem: currId should go to 0, and the viewport
	// should scroll up so that item 0 is visible.
	// Before fix: topBoundId stays at 1, item 0 is above the viewport.
	// After fix: topBoundId becomes 0, item 0 is at the top of the viewport.
	m.PrevItem()

	if m.GetCurrItem() != 0 {
		t.Fatalf("expected currId=0, got %d", m.GetCurrItem())
	}
	if m.topBoundId != 0 {
		t.Errorf(
			"PrevItem did not scroll up at the top boundary: expected topBoundId=0, got %d (item 0 is above the visible viewport)",
			m.topBoundId,
		)
	}
}

func TestNextItemScrollsAtBottomBound(t *testing.T) {
	// Verify NextItem scrolling works symmetrically
	m := newTestModel(testModelOpts{numItems: 10, viewportHeight: 5, itemHeight: 1})

	// Navigate down to bottomBoundId (4)
	for range 4 {
		m.NextItem()
	}

	if m.GetCurrItem() != 4 {
		t.Fatalf("expected currId=4, got %d", m.GetCurrItem())
	}
	if m.topBoundId != 0 {
		t.Fatalf("expected topBoundId=0 (no scroll yet), got %d", m.topBoundId)
	}

	// One more NextItem at the bottom boundary should scroll
	m.NextItem()

	if m.GetCurrItem() != 5 {
		t.Fatalf("expected currId=5, got %d", m.GetCurrItem())
	}
	if m.topBoundId != 1 {
		t.Errorf(
			"NextItem did not scroll down at the bottom boundary: expected topBoundId=1, got %d",
			m.topBoundId,
		)
	}
}

func TestPrevItemAtFirstItem(t *testing.T) {
	// Pressing up when already at the first item should stay at 0
	// and should not corrupt viewport bounds.
	m := newTestModel(testModelOpts{numItems: 10, viewportHeight: 5, itemHeight: 1})

	m.PrevItem()

	if m.GetCurrItem() != 0 {
		t.Errorf("expected currId=0, got %d", m.GetCurrItem())
	}
	if m.topBoundId != 0 {
		t.Errorf("expected topBoundId=0 (should not scroll), got %d", m.topBoundId)
	}
	if m.bottomBoundId != 4 {
		t.Errorf("expected bottomBoundId=4 (should not scroll), got %d", m.bottomBoundId)
	}
}

func TestNextItemAtLastItem(t *testing.T) {
	// Pressing down when already at the last item should stay at last
	m := newTestModel(testModelOpts{numItems: 10, viewportHeight: 5, itemHeight: 1})

	for range 20 {
		m.NextItem()
	}

	if m.GetCurrItem() != 9 {
		t.Errorf("expected currId=9, got %d", m.GetCurrItem())
	}
}

// SetNumItems runs on every row rebuild -- which used to mean every message, a
// scroll event included. It must keep the visible window where the scroll left
// it. Recomputing bottomBoundId as if the list were at the top (while
// topBoundId and the scroll offset stayed put) shrank the window, so after a
// direction reversal the next scroll-down moved the LIST instead of the cursor:
// the highlight froze mid-screen and the rows slid underneath it.
func TestSetNumItems_KeepsTheScrolledWindow(t *testing.T) {
	m := newTestModel(testModelOpts{numItems: 30, viewportHeight: 5, itemHeight: 1})

	for range 12 { // past a page: window scrolled to items 8..12, cursor at 12
		m.NextItem()
	}
	for range 3 { // back up within the window: cursor 9, window unchanged
		m.PrevItem()
	}
	require.Equal(t, 9, m.GetCurrItem())
	require.Equal(t, 8, m.topBoundId)

	m.SetNumItems(30) // a rebuild with the same rows

	m.NextItem() // cursor 10: still inside the window, so nothing may scroll
	require.Equal(t, 10, m.GetCurrItem())
	require.Equal(t, 8, m.topBoundId, "the window must not scroll while the cursor is inside it")
	require.Equal(t, 12, m.bottomBoundId)
}

// A list that shrank under a scrolled window must not leave the window pointing
// past its end.
func TestSetNumItems_ClampsTheWindowWhenTheListShrinks(t *testing.T) {
	m := newTestModel(testModelOpts{numItems: 30, viewportHeight: 5, itemHeight: 1})
	for range 20 {
		m.NextItem()
	}

	m.SetNumItems(10)

	require.LessOrEqual(t, m.bottomBoundId, 9, "the window must end inside the list")
	require.Equal(t, 4, m.bottomBoundId-m.topBoundId, "the window keeps its height")
	require.LessOrEqual(t, m.GetCurrItem(), 9, "the cursor must stay inside the list")
	require.GreaterOrEqual(t, m.GetCurrItem(), m.topBoundId, "the cursor must stay inside the window")
}
