package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func composeRegions(mid string, midWidth int, left string, leftWidth int, right string, rightWidth int, width int) string {
	leftLines := regionLines(left)
	rightLines := regionLines(right)
	midLines := strings.Split(mid, "\n")

	// The separator is a column of its own, charged only for a side that has
	// content.
	gap := 0
	if leftWidth > 0 {
		gap++
	}
	if rightWidth > 0 {
		gap++
	}
	midWidth = max(1, min(midWidth, width-leftWidth-rightWidth-gap))

	n := max(len(midLines), len(leftLines), len(rightLines))
	lines := make([]string, n)
	for i := range lines {
		var b strings.Builder
		if leftWidth > 0 {
			b.WriteString(padTo(clampLine(pick(leftLines, i), leftWidth), leftWidth))
			b.WriteString(" ")
		}
		b.WriteString(padTo(clampLine(pick(midLines, i), midWidth), midWidth))
		if rightWidth > 0 {
			b.WriteString(" ")
			b.WriteString(padTo(clampLine(pick(rightLines, i), rightWidth), rightWidth))
		}
		lines[i] = clampLine(b.String(), width)
	}
	return strings.Join(lines, "\n")
}

// regionLines splits a rendered region into lines, treating an absent region as
// no lines at all.
func regionLines(region string) []string {
	if region == "" {
		return nil
	}
	return strings.Split(strings.TrimRight(region, "\n"), "\n")
}

// pick returns line i, or an empty string past the end.
func pick(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return ""
}

// maxLineWidth is the widest visible line in a set.
func maxLineWidth(lines []string) int {
	w := 0
	for _, l := range lines {
		if v := lipgloss.Width(l); v > w {
			w = v
		}
	}
	return w
}

// promptGlyph marks the command line, both where it is typed and where it is
// echoed back into the transcript.
//
// It is one constant because the two must agree: a transcript whose commands
// are echoed under a different marker than the one the user types under reads
// as two different things, and a test that hard-codes one of them silently
// stops checking the other.
const promptGlyph = "❯ "

// padTo pads a line to a visible width.
func padTo(line string, width int) string {
	if gap := width - lipgloss.Width(line); gap > 0 {
		return line + strings.Repeat(" ", gap)
	}
	return line
}

// renderHeader draws the tool identity and session state on one line.
//
// The identity leads: a brand mark and the tool's name, with the version
// trailing faintly. The state is a status pill on the right, written with both
// a symbol and a word so a monochrome terminal still reads it. Colour in the
// resting header is the brand mark and the pill alone; everything between is
// text.
