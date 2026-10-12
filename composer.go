package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m model) renderForm() string {
	f := m.form
	if f == nil {
		return m.renderComposer()
	}
	var b strings.Builder
	b.WriteString(ground(m.theme.Inset, m.theme.Rule.Render(strings.Repeat("─", max(1, m.width))), m.width))
	b.WriteString("\n")
	// The form takes the composer's place, so it takes the mode bar too. Without
	// it the keyboard's location is stated only in the footer hint, which is the
	// one part of the form that scrolls out of the way.
	b.WriteString(m.renderModeBar())
	b.WriteString("\n")
	title := m.theme.Title.Render("FILL IN  " + f.Capability.Name)
	b.WriteString(ground(m.theme.Inset, clampLine(title, m.width), m.width))
	b.WriteString("\n")

	for i, fl := range f.fields {
		marker := "  "
		style := m.theme.Value
		if i == f.focus {
			marker = "> "
			style = m.theme.Command
		}
		label := fl.param.Name
		if fl.param.Required {
			label += "*"
		}
		// The label is padded so the values line up, but the value is what
		// matters, so a long label is truncated rather than the value.
		value := fl.value
		if value == "" && fl.param.Default == "" {
			value = m.theme.Detail.Render("(unset)")
		}
		row := marker + m.theme.Label.Render(pad(truncate(label, 18), 19)) + style.Render(value)
		if fl.cursor > 0 && i == f.focus {
			row += "▏"
		}
		// The focused field is a filled box, and it has to be a solid one: the
		// value's own styling ends in a reset, so the field is grounded with the
		// fill re-established after every reset rather than rendered with a bare
		// Render that would leave the box's tail unpainted. The remaining rows
		// sit on the frame's ground.
		row = padTo(clampLine(row, m.width), m.width)
		if i == f.focus {
			row = ground(m.theme.Inset, ground(m.theme.Surface, row, m.width), m.width)
		} else {
			row = ground(m.theme.Inset, row, m.width)
		}
		b.WriteString(row)
		b.WriteString("\n")
	}

	if f.Err() != "" {
		b.WriteString(ground(m.theme.Inset, clampLine(m.theme.Failed.Render("  "+f.Err()), m.width), m.width))
		b.WriteString("\n")
	}
	// The keys are drawn with the shared key hint, so the form's footer and the
	// rest of the interface describe keys the same way.
	hint := "  " + keyHint(m.theme, "enter", "run", m.width-2) +
		"  " + keyHint(m.theme, "tab", "next field", m.width-2) +
		"  " + keyHint(m.theme, "esc", "cancel", m.width-2)
	b.WriteString(ground(m.theme.Inset, clampLine(hint, m.width), m.width))
	return b.String()
}

// composeRegions places side regions beside the transcript at a given width.
//
// The column widths are given rather than measured from the rendered text. A
// viewport's lines are exactly as wide as the viewport, so measuring the middle
// column yields the full terminal width and then clamps it, which truncates
// every transcript line with an ellipsis in the space the region has already
// vacated. Measuring the regions is equally wrong: a row shorter than its
// column would silently narrow it.
//
// The three widths plus their separators must tile the terminal exactly. If they
// cannot, the transcript loses the difference, because it is the one column that
// reflows rather than the one that wraps mid-word.
func (m model) renderStrip() string {
	if !m.following {
		below := m.viewport.TotalLineCount() - (m.viewport.YOffset + m.viewport.Height)
		if below > 0 {
			label := fmt.Sprintf("↓ %s · ctrl+end to follow", plural(below, "new line", "new lines"))
			return m.theme.Hint.Render("  " + label)
		}
	}
	return ""
}

// renderComposer draws the bottom input bar.
//
// The composer is the one element that must never move, because it is the only
// part of the interface the user is always addressing. Everything above it
// scrolls.
func (m model) renderComposer() string {
	// The bar is visually distinct from the transcript without being a boxed
	// panel: a rule above it, a mode bar naming where the keyboard is, and a
	// green prompt are enough.
	var b strings.Builder
	b.WriteString(ground(m.theme.Inset, m.theme.Rule.Render(strings.Repeat("─", max(1, m.width))), m.width))
	b.WriteString("\n")
	b.WriteString(m.renderModeBar())
	b.WriteString("\n")

	line := m.input.View()
	if hint := m.hint(); hint != "" {
		hint = m.theme.Hint.Render(hint)
		// The input pads itself out to its full width, so the hint cannot be
		// placed by measuring the rendered line -- that measures the padding
		// too, always reports the terminal as full, and silently drops the
		// hints on every terminal. The space between the cursor and the hint is
		// measured from what was actually typed instead.
		gap := m.width - m.composerTextWidth() - lipgloss.Width(hint)
		if gap >= 2 {
			line = strings.TrimRight(line, " ") + strings.Repeat(" ", gap) + hint
		}
	}
	// The input row is the one surface the operator is touching, so it takes the
	// top of the ladder rather than sharing Surface with the header and the
	// panels. Drawn on Surface -- which is what it did -- the field the caret
	// sits in is indistinguishable from a panel being read, and the eye has no
	// place to rest while typing.
	//
	// The fill goes on through ground rather than a bare Render. The typed text
	// and the hint both end with a reset, and a reset clears the background, so
	// a bare Render would colour the characters and leave the rest of the field
	// -- the empty width a short command does not fill -- on the terminal's own
	// background. That is the "coloured text rather than a field" the field
	// looked like before.
	line = padTo(clampLine(line, m.width), m.width)
	b.WriteString(ground(m.theme.Inset, ground(m.theme.Raised, line, m.width), m.width))
	return b.String()
}

// composerTextWidth is the width the typed text actually occupies: the prompt,
// the value, and the cursor.
//
// This is what the composer hint is laid out against. m.input.View() cannot be
// used for it, because the widget fills its configured width with padding and a
// measurement of it says the line is always as wide as the terminal.
func (m model) composerTextWidth() int {
	w := lipgloss.Width(m.input.Prompt) + 1 // the cursor always occupies a cell
	if v := m.input.Value(); v != "" {
		return w + lipgloss.Width(v)
	}
	return w + lipgloss.Width(m.input.Placeholder)
}

// hint returns the context-sensitive key hint on the right of the composer.
//
// The hint is how a key the interface has just added stays discoverable without
// a manual. F1 is mentioned only while the region is open, and the running hint
// leads with the fact that something is in flight, which matters more on a wide
// terminal than the registry is.
func (m model) hint() string {
	// A form takes the keyboard, so the hint describes the form's keys rather
	// than the composer's: telling someone to press tab to complete when tab
	// moves between fields is worse than saying nothing.
	if m.form != nil {
		return "tab next · ↑↓ field · esc cancel"
	}
	if m.regionFocus {
		return "↑↓ move · enter/space expand · esc back"
	}
	if m.running {
		// The queue is the most important thing to say while work is in
		// flight: the operator may have typed ahead, and knowing a second
		// command is waiting is what stops a queued run from looking dropped.
		if n := len(m.queued); n > 0 {
			return fmt.Sprintf("ctrl+c stop · %d queued", n)
		}
		return "ctrl+c stop · ctrl+e export"
	}
	if m.showCapabilities {
		return "F1 hide · form <id> · tab complete"
	}
	return "F1 capabilities · ctrl+e export · tab complete"
}

// transcriptLines renders the whole session as terminal lines.
//
// The transcript is a flat list of lines rather than a set of widgets, which is
// what lets the viewport scroll it and the composer stay put. A completed
// command collapses to a compact summary with its output expandable; a running
// one shows its live state.
