package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

// A styled frame: red "hello", default " world", then a plain second line.
const testFrame = "\x1b[31mhello\x1b[0m world\nsecond line here"

func sel(ax, ay, cx, cy int) textSelection {
	return textSelection{active: true, anchorX: ax, anchorY: ay, cursorX: cx, cursorY: cy}
}

// Highlighting is a presentation pass. It must never change what characters are
// on screen, nor how wide any line is -- otherwise the layout shifts under the
// user mid-drag.
func TestHighlightFrame_PreservesTextAndWidth(t *testing.T) {
	got := highlightFrame(testFrame, sel(0, 0, 4, 0))

	require.Equal(t, ansi.Strip(testFrame), ansi.Strip(got),
		"highlighting must not change the visible characters")

	origLines := strings.Split(testFrame, "\n")
	gotLines := strings.Split(got, "\n")
	require.Len(t, gotLines, len(origLines))
	for i := range origLines {
		require.Equal(t, ansi.StringWidth(origLines[i]), ansi.StringWidth(gotLines[i]),
			"line %d width changed", i)
	}

	require.Contains(t, got, reverseOn, "selected span should be reverse-video")
}

func TestHighlightFrame_InactiveIsUntouched(t *testing.T) {
	s := sel(0, 0, 4, 0)
	s.active = false
	require.Equal(t, testFrame, highlightFrame(testFrame, s))
}

func TestSelectedText_SingleLine(t *testing.T) {
	// cells 0..4 inclusive of the cell under the pointer
	require.Equal(t, "hello", selectedText(testFrame, sel(0, 0, 4, 0)))
}

func TestSelectedText_StripsStylingAndTrailingPadding(t *testing.T) {
	frame := "\x1b[7mword\x1b[0m      \nnext"
	require.Equal(t, "word", selectedText(frame, sel(0, 0, 9, 0)))
}

// A selection flows across the end of a line, like a terminal's own.
func TestSelectedText_MultiLineIsLinearNotRectangular(t *testing.T) {
	require.Equal(t, "world\nsecond", selectedText(testFrame, sel(6, 0, 5, 1)))
}

// Dragging up/left must select the same span as dragging down/right.
func TestSelectedText_ReverseDragIsSymmetric(t *testing.T) {
	forward := selectedText(testFrame, sel(6, 0, 5, 1))
	backward := selectedText(testFrame, sel(5, 1, 6, 0))
	require.Equal(t, forward, backward)
}

func TestSelection_IsEmptyOnlyWhenPointerNeverLeftTheCell(t *testing.T) {
	require.True(t, sel(3, 2, 3, 2).isEmpty())
	require.False(t, sel(3, 2, 4, 2).isEmpty())
	require.False(t, sel(3, 2, 3, 3).isEmpty())
}

// Coordinates past the end of a short line must clamp, not panic or over-read.
func TestSelection_LineBoundsClamp(t *testing.T) {
	s := sel(0, 0, 999, 0)
	lo, hi, ok := s.lineBounds(0, 11)
	require.True(t, ok)
	require.Equal(t, 0, lo)
	require.Equal(t, 11, hi)

	_, _, ok = s.lineBounds(5, 11) // line outside the selection
	require.False(t, ok)
}

func TestSelectedText_EmptyFrameOrInactive(t *testing.T) {
	require.Equal(t, "", selectedText("", sel(0, 0, 4, 0)))

	s := sel(0, 0, 4, 0)
	s.active = false
	require.Equal(t, "", selectedText(testFrame, s))
}
