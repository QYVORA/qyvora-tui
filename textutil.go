package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func clampPercent(p float64) float64 {
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

// contentWidth is the usable width of the transcript area, inside its indent.
//
// It follows the viewport rather than the terminal. The viewport is what
// actually renders the transcript, and it is narrower than the terminal while a
// side region is drawn; measuring against the terminal is how a block's lines
// wrap to a width the viewport cannot hold and then come back truncated with an
// ellipsis where the region now is.
func (m model) contentWidth() int {
	return max(24, m.viewport.Width-2)
}

// clampLine shortens a rendered line to fit the terminal.
//
// Left as a visible truncation rather than a wrap: a wrapped tree line breaks
// the alignment that makes the shape readable, and the information is still
// available in the expanded event view.
func clampLine(line string, width int) string {
	if width <= 0 || lipgloss.Width(line) <= width {
		return line
	}
	r := []rune(stripANSI(line))
	if len(r) > width {
		if width == 1 {
			return string(r[:1])
		}
		return string(r[:width-1]) + "…"
	}
	return line
}

// truncate shortens a value for single-line display. Multi-line values are
// collapsed rather than dropped, so a value never vanishes entirely.
func truncate(s string, width int) string {
	if width <= 0 || s == "" {
		return s
	}
	s = strings.ReplaceAll(s, "\n", " ")
	// Count runes: event payloads are frequently non-ASCII, and byte slicing
	// would cut a multi-byte character in half.
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	if width <= 1 {
		return string(r[:width])
	}
	return string(r[:width-1]) + "…"
}

// pad right-pads a label to a fixed width, counting display width.
func pad(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// applyLayout resolves the geometry for a terminal size and rebuilds the
// viewport to match.
//
// The layout is resolved once here and read by everything else, so a region
// and the transcript can never disagree about how wide they are. Regions are
// offered rather than imposed: a tool with no capabilities and nothing running
// is given a full-width transcript even on a wide terminal, because an empty
// column is worse than no column.
