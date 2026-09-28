package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Region is a side panel: a titled column beside the transcript.
//
// A region is a component in the sense the architecture needs: it knows its
// width, it takes its colours from the theme rather than holding any of its
// own, and it can be tested without a terminal or a model. It is not a widget
// framework, and it deliberately does not grow into one.
//
// Rendering a region to an empty string is the normal case, not an error. A
// tool with no capabilities renders no navigation region, and a layout that
// reserved space for it would be worse than one that did not.
type Region struct {
	// Title is the region's heading.
	Title string

	// Rows are the region's content, already formatted. A region composes
	// rows rather than owning state.
	Rows []string

	// Width is the column width the region is rendered at.
	Width int

	// Empty is shown in place of Rows when there is nothing to display, so the
	// region explains itself instead of being a blank column.
	Empty string

	// Focused marks the region as holding keyboard focus, for tools that route
	// keys to a region. A region that cannot be focused leaves it false.
	Focused bool
}

// NewRegion builds a region at a width.
func NewRegion(title string, width int) *Region {
	return &Region{Title: title, Width: max(8, width)}
}

// Add appends a row, ignoring any wider than the region. Clamping here rather
// than at render time keeps the row list meaningful for tests.
func (r *Region) Add(row string) {
	if r == nil || row == "" {
		return
	}
	r.Rows = append(r.Rows, clampLine(row, r.Width))
}

// SetRows replaces the region's content.
func (r *Region) SetRows(rows []string) {
	if r == nil {
		return
	}
	r.Rows = r.Rows[:0]
	for _, row := range rows {
		r.Add(row)
	}
}

// Len reports how many rows the region holds, which is what a layout uses to
// decide whether a region is worth drawing at all.
func (r *Region) Len() int {
	if r == nil {
		return 0
	}
	return len(r.Rows)
}

// Render draws the region: a title, a rule, and its rows.
func (r *Region) Render(t Theme) string {
	if r == nil || r.Width < minNavigationWidth {
		return ""
	}
	var b strings.Builder
	title := r.Title
	if r.Focused {
		title = t.Selection.Render(r.Title)
	} else {
		title = t.Label.Render(r.Title)
	}
	b.WriteString(clampLine(title, r.Width))
	b.WriteString("\n")
	b.WriteString(t.Border.Render(strings.Repeat("─", r.Width)))
	b.WriteString("\n")

	if len(r.Rows) == 0 {
		if r.Empty != "" {
			b.WriteString(t.Detail.Render(clampLine(r.Empty, r.Width)))
			b.WriteString("\n")
		}
		return b.String()
	}
	for _, row := range r.Rows {
		b.WriteString(row)
		b.WriteString("\n")
	}
	return b.String()
}

// Boxed renders the region with a vertical rule on its inner edge, which is how
// a side region reads as a panel rather than as more transcript.
//
// The rule is drawn as a separate column so the transcript's own width is not
// disturbed: a region that ate a character of every transcript line would make
// the session harder to read for no gain.
func (r *Region) Boxed(t Theme, side int) string {
	body := r.Render(t)
	if strings.TrimSpace(body) == "" {
		return ""
	}
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	const rule = "│"
	// side is negative for a region on the left, where the rule belongs on the
	// region's right edge -- the edge facing the transcript. A region on the
	// right puts it on its left edge, for the same reason.
	innerEdgeLast := side >= 0
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		padded := line
		if w := lipgloss.Width(line); w < r.Width {
			padded += strings.Repeat(" ", r.Width-w)
		}
		if innerEdgeLast {
			out = append(out, padded+" "+t.Border.Render(rule))
			continue
		}
		out = append(out, t.Border.Render(rule)+" "+padded)
	}
	return strings.Join(out, "\n")
}

// Row is one line of a region, built from labelled parts so a component does
// not have to hand-align text.
//
// A region is narrow, and hand-aligned text at a narrow width is how columns
// stop lining up. A Row measures its own parts and pads them to fit.
type Row struct {
	// Marker is an optional leading glyph, e.g. a status symbol or a tree
	// branch. It is never truncated, because a status symbol is the one thing
	// in the row that carries meaning.
	Marker string

	// Key is the leading label, e.g. a capability name or a command.
	Key string

	// Value is the trailing detail, dropped before the key when space runs
	// out. Losing detail is preferable to losing the label that identifies the
	// row.
	Value string
}

// String renders the row at a width.
func (r Row) String(width int) string {
	marker := r.Marker
	if marker != "" {
		marker += " "
	}
	key := r.Key
	if width <= 0 {
		return clampLine(marker+key, 0)
	}
	avail := width - len([]rune(marker))
	if avail <= 0 {
		return clampLine(marker+key, width)
	}
	if r.Value == "" {
		return clampLine(marker+key, width)
	}
	// Reserve a space between the two parts, and drop the value entirely if
	// what is left cannot hold a readable key.
	keyWidth := avail - 1
	if keyWidth < 6 {
		return clampLine(marker+key, width)
	}
	if w := len([]rune(key)); w > keyWidth {
		key = truncate(key, keyWidth)
	}
	// The gap is what is left after the key, the value and the single space
	// that separates them. It is computed from both parts, not just the key:
	// budgeting only for the key is what makes a row overflow the width it was
	// asked to fit, and then get silently clamped, losing the value the row was
	// careful enough to fit.
	value := r.Value
	gap := avail - len([]rune(key)) - 1 - len([]rune(value))
	if gap < 1 {
		// No room for both. The value goes rather than the key, and if the
		// value is what is left over entirely then only the key can be shown.
		if avail-len([]rune(key))-1 < 4 {
			return clampLine(marker+key, width)
		}
		value = ""
		gap = avail - len([]rune(key)) - 1
	}
	line := marker + key + strings.Repeat(" ", gap+1) + value
	return clampLine(line, width)
}

// keyHint renders a key and its description for a footer or a region row,
// truncating the description rather than the key.
func keyHint(t Theme, key, desc string, width int) string {
	if key == "" {
		return ""
	}
	k := t.Badge.Render(key)
	if desc == "" {
		return clampLine(k, width)
	}
	avail := width - lipgloss.Width(k) - 1
	if avail < 8 {
		return clampLine(k, width)
	}
	return clampLine(k+" "+t.Hint.Render(truncate(desc, avail)), width)
}
