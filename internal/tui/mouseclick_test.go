package tui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const prURL = "https://github.com/o/r/pull/12"

func TestSubPageURL(t *testing.T) {
	require.Equal(t, prURL+"/checks", subPageURL(prURL, "checks"))
	require.Equal(t, prURL+"/files", subPageURL(prURL+"/", "files"))
	require.Equal(t, "", subPageURL("", "checks"), "no URL, nothing to open")
}

// The whole point of the refinement: a bare click selects, it does not open.
func TestRowClickAction_BareClickOpensNothing(t *testing.T) {
	require.Equal(t, "", rowClickAction(prURL, rowHit{}, false))
}

func TestRowClickAction_DoubleClickOpensThePR(t *testing.T) {
	require.Equal(t, prURL, rowClickAction(prURL, rowHit{}, true))
}

func TestRowClickAction_NumberOpensThePRWithoutDoubleClick(t *testing.T) {
	require.Equal(t, prURL, rowClickAction(prURL, rowHit{number: true}, false))
}

func TestRowClickAction_IconsOpenTheirSubPages(t *testing.T) {
	require.Equal(t, prURL+"/checks", rowClickAction(prURL, rowHit{ci: true}, false))
	require.Equal(t, prURL+"/files", rowClickAction(prURL, rowHit{review: true}, false))
	require.Equal(t, prURL, rowClickAction(prURL, rowHit{comments: true}, false),
		"comments open the conversation, which is the PR page itself")
}

func TestRowClickAction_NoURLOpensNothing(t *testing.T) {
	require.Equal(t, "", rowClickAction("", rowHit{number: true}, true))
}

func TestRowHit_Targeted(t *testing.T) {
	require.False(t, rowHit{}.targeted())
	require.True(t, rowHit{ci: true}.targeted())
	require.True(t, rowHit{number: true}.targeted())
}

func TestRegisterRowClick_PairsWithinTheWindow(t *testing.T) {
	base := time.Unix(1700000000, 0)
	now := base
	orig := nowFunc
	nowFunc = func() time.Time { return now }
	t.Cleanup(func() { nowFunc = orig })

	m := &Model{lastClickRow: -1}

	require.False(t, m.registerRowClick(2), "first click is never a double-click")

	now = base.Add(100 * time.Millisecond)
	require.True(t, m.registerRowClick(2), "second click inside the window pairs")

	// The pair is consumed: a third click starts a fresh one.
	now = base.Add(150 * time.Millisecond)
	require.False(t, m.registerRowClick(2), "a third click must not re-open")
}

func TestRegisterRowClick_TooSlowIsTwoSingleClicks(t *testing.T) {
	base := time.Unix(1700000000, 0)
	now := base
	orig := nowFunc
	nowFunc = func() time.Time { return now }
	t.Cleanup(func() { nowFunc = orig })

	m := &Model{lastClickRow: -1}
	require.False(t, m.registerRowClick(1))

	now = base.Add(doubleClickWindow + time.Millisecond)
	require.False(t, m.registerRowClick(1), "outside the window is a fresh single click")
}

func TestRegisterRowClick_DifferentRowDoesNotPair(t *testing.T) {
	base := time.Unix(1700000000, 0)
	now := base
	orig := nowFunc
	nowFunc = func() time.Time { return now }
	t.Cleanup(func() { nowFunc = orig })

	m := &Model{lastClickRow: -1}
	require.False(t, m.registerRowClick(0))

	now = base.Add(50 * time.Millisecond)
	require.False(t, m.registerRowClick(1), "clicking a different row is not a double-click")
}

// A zero-value Model must not treat its very first click on row 0 as a pair.
func TestRegisterRowClick_ZeroValueModelIsSafe(t *testing.T) {
	m := &Model{} // lastClickRow == 0, lastClickAt == zero time
	require.False(t, m.registerRowClick(0))
}
