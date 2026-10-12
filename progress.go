package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func countsLen(s string) int { return lipgloss.Width(s) }

// minBarCells is the narrowest a progress bar is drawn.
//
// A bar of four cells can only report 0, 25, 50, 75 or 100 percent, so it is a
// decoration that suggests a reading it cannot give. Below this width the
// percentage is drawn on its own, which is the honest version of the same fact.
const minBarCells = 8

// shortCount abbreviates a count to its magnitude, which is the part that is
// worth a narrow column: 812,447 becomes "812k" and 1,000,000 becomes "1.0M".
//
// One decimal below a thousand is not worth the column, so 9,500 becomes "9.5k"
// and 950 becomes "950". Rounding up past the next unit would report more work
// than the tool said it had, which is the one direction this must not err in.
func shortCount(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	// Tenths are computed with integer division so that a value just below the
	// next unit abbreviates down rather than rounding up into it. Formatting the
	// quotient as a float and trimming a trailing zero turns 9,999 into "10.0k",
	// which reports more work than the tool said it had and lands on the wrong
	// unit besides.
	var out string
	switch {
	case n < 1000:
		out = strconv.FormatInt(n, 10)
	case n < 1000000:
		if tenths := n / 100; tenths >= 100 {
			out = strconv.FormatInt(tenths/10, 10) + "k"
		} else {
			out = unitCount(tenths, "k")
		}
	case n < 1000000000:
		if tenths := n / 100000; tenths >= 100 {
			out = strconv.FormatInt(tenths/10, 10) + "M"
		} else {
			out = unitCount(tenths, "M")
		}
	default:
		if tenths := n / 100000000; tenths >= 100 {
			out = strconv.FormatInt(tenths/10, 10) + "B"
		} else {
			out = unitCount(tenths, "B")
		}
	}
	if neg {
		return "-" + out
	}
	return out
}

// unitCount builds a one-decimal abbreviation from tenths of the unit, dropping
// the decimal when it would be a bare zero. The unit is appended separately so
// that "1.0M" is recognised as the round "1M"; testing for a ".0" suffix on the
// assembled string never matches, because the suffix is ".0M".
func unitCount(tenths int64, unit string) string {
	whole, frac := tenths/10, tenths%10
	if frac == 0 {
		return strconv.FormatInt(whole, 10) + unit
	}
	return strconv.FormatInt(whole, 10) + "." + strconv.FormatInt(frac, 10) + unit
}

// commas groups a count in thousands, the way a terminal tool prints one. An
// ungrouped 1000000 is six digits of which the operator has to count the digits
// to read the magnitude, and the whole point of showing the count is the
// magnitude.
func commas(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

// renderSummary is the one-line outcome of a finished execution: what it was,
// how long it took, and what it left behind.
//
// It answers "what did that do?" -- the question an operator asks of every
// command -- in a single line. The status and the tally used to be two lines
// that repeated the finding count between them, spending two rows to say less
// than one does. A finished run is now one row and the count appears once.
//
// The finding count is drawn in the colour of the worst severity present, so a
// glance at a finished run reports how bad it was and not only how much.
