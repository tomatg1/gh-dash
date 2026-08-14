package tui

import (
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"
)

// copyToClipboard is a seam so tests don't scribble on the user's clipboard.
var copyToClipboard = clipboard.WriteAll

// A press that moves this little is a click, not a drag. Trackpad taps almost
// always jitter a cell or two; without a threshold every click would register a
// stray one-character selection and leave a lingering highlight on the row.
// Vertical counts double: rows are two cells tall, so one row of travel is a
// bigger intent than one column.
const (
	dragThresholdX = 3
	dragThresholdY = 2
)

// pastDragThreshold reports whether the pointer has moved far enough from the
// press point to count as a drag rather than a click.
func pastDragThreshold(anchorX, anchorY, x, y int) bool {
	dx := x - anchorX
	if dx < 0 {
		dx = -dx
	}
	dy := y - anchorY
	if dy < 0 {
		dy = -dy
	}

	return dx >= dragThresholdX || dy >= dragThresholdY
}

// Reverse video. Explicit SGR rather than lipgloss so the span's width is
// provably unchanged -- these are zero-width control sequences.
const (
	reverseOn  = "\x1b[7m"
	reverseOff = "\x1b[27m"
)

// textSelection is a linear, terminal-style selection over the rendered frame:
// it runs from the cell where the button went down to the cell the pointer is
// on now, flowing across line ends rather than selecting a rectangle.
//
// gh-dash has to draw this itself. The moment a program enables mouse
// reporting, the terminal hands drag events to the program and stops drawing
// its own selection -- so "drag to select" and "click to open" can only coexist
// if the program tells the two apart and renders the selection on its own.
type textSelection struct {
	active  bool
	anchorX int // cell where the button went down
	anchorY int
	cursorX int // cell the pointer is on now
	cursorY int
}

// ordered returns the bounds in reading order, inclusive of both endpoints,
// regardless of which way the user dragged.
func (s textSelection) ordered() (x1, y1, x2, y2 int) {
	if s.anchorY < s.cursorY || (s.anchorY == s.cursorY && s.anchorX <= s.cursorX) {
		return s.anchorX, s.anchorY, s.cursorX, s.cursorY
	}

	return s.cursorX, s.cursorY, s.anchorX, s.anchorY
}

// isEmpty reports whether the pointer never left the cell it pressed on.
func (s textSelection) isEmpty() bool {
	return s.anchorX == s.cursorX && s.anchorY == s.cursorY
}

// lineBounds gives the [lo, hi) cell range selected on line y of a frame whose
// line y is lineWidth cells wide.
func (s textSelection) lineBounds(y, lineWidth int) (lo, hi int, ok bool) {
	x1, y1, x2, y2 := s.ordered()
	if y < y1 || y > y2 {
		return 0, 0, false
	}

	lo, hi = 0, lineWidth
	if y == y1 {
		lo = x1
	}
	if y == y2 {
		hi = x2 + 1 // the cell under the pointer is part of the selection
	}

	lo = clampSel(lo, 0, lineWidth)
	hi = clampSel(hi, 0, lineWidth)
	if lo >= hi {
		return 0, 0, false
	}

	return lo, hi, true
}

func clampSel(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}

	return v
}

// highlightFrame reverse-videos the selected cells of an already-rendered frame.
//
// The selected span is stripped of its own styling before being reversed: an
// SGR reset nested inside the span would otherwise cancel the reverse attribute
// partway through. Stripping removes only zero-width escape sequences, so the
// span's cell width -- and therefore the whole layout -- is unchanged.
func highlightFrame(frame string, s textSelection) string {
	if !s.active || frame == "" {
		return frame
	}

	lines := strings.Split(frame, "\n")
	for y, line := range lines {
		width := ansi.StringWidth(line)
		lo, hi, ok := s.lineBounds(y, width)
		if !ok {
			continue
		}

		lines[y] = ansi.Cut(line, 0, lo) +
			reverseOn + ansi.Strip(ansi.Cut(line, lo, hi)) + reverseOff +
			ansi.Cut(line, hi, width)
	}

	return strings.Join(lines, "\n")
}

// selectedText is the plain text under the selection, ready for the clipboard.
// Trailing padding is trimmed per line, the way a terminal's own selection does.
func selectedText(frame string, s textSelection) string {
	if !s.active || frame == "" {
		return ""
	}

	lines := strings.Split(frame, "\n")
	_, y1, _, y2 := s.ordered()

	out := make([]string, 0, y2-y1+1)
	for y := y1; y <= y2; y++ {
		if y < 0 || y >= len(lines) {
			continue
		}

		line := lines[y]
		lo, hi, ok := s.lineBounds(y, ansi.StringWidth(line))
		if !ok {
			out = append(out, "")
			continue
		}

		out = append(out, strings.TrimRight(ansi.Strip(ansi.Cut(line, lo, hi)), " "))
	}

	return strings.Trim(strings.Join(out, "\n"), "\n ")
}
