package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func (m model) renderHeader() string {
	right := m.statusPill()

	// The banner replaces the header's own identity row rather than sitting
	// above it. Drawing both would name the tool twice, and the wordmark is the
	// version of the name that says the same thing at a glance, so the text row
	// stands in only when the art has nothing to offer.
	if art := m.renderBanner(); len(art) > 0 {
		return m.bannerWithStatus(art, right)
	}

	title := m.cfg.Title
	if title == "" {
		title = "QYVORA"
	}
	// The identity is set in the accent and the version is dimmed beside it,
	// which is what makes the row read as "who this is" and "which build" rather
	// than as one run of text. It also matches the plain-text banner exactly, so
	// the same tool looks like itself whether it is drawing a terminal or
	// writing to a pipe.
	left := m.theme.Title.Render("▇ ") + m.theme.Title.Render(title)
	if m.cfg.Version != "" {
		left += "  " + m.theme.Version.Render(m.cfg.Version)
	}

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		// Too narrow to sit side by side. The identity matters more than the
		// status, so the status gives way rather than both being clipped.
		return m.band(clampLine(left, m.width))
	}
	return m.band(left + strings.Repeat(" ", gap) + right)
}

// bannerHeight is how many rows the banner draws in the header, zero when it
// draws nothing.
//
// The header is charged this from the same place the transcript's height is
// computed, so a banner cannot push the composer's last row off the bottom of
// the terminal. A banner that is allowed to overrun the viewport is the one
// piece of furniture in the interface that would make the input unreachable.
// bannerHeight is the header's row count when a banner is configured: the art
// rows, plus a row for the status pill when it does not fit beside the art.
//
// The pill's row has to be counted here as well as drawn, or the header claims
// fewer rows than it occupies. That is not a cosmetic off-by-one: chromeHeight
// is the difference between the terminal and the viewport, so undercounting it
// gives the transcript rows that were never there, and View writes one row more
// than the terminal has. The extra row scrolls the frame, so the input row the
// operator is typing into is the row that disappears.
func (m model) bannerHeight() int {
	art := m.renderBanner()
	if len(art) == 0 {
		return 0
	}
	if m.statusBesideBanner(art) {
		return len(art)
	}
	return len(art) + 1 // the status pill on its own row below the art
}

// statusBesideBanner reports whether the status pill has room beside the art.
func (m model) statusBesideBanner(art []string) bool {
	artW := 0
	for _, l := range art {
		if n := lipgloss.Width(l); n > artW {
			artW = n
		}
	}
	return artW+1+lipgloss.Width(m.statusPill()) <= m.width
}

// renderBanner draws the tool's banner in the theme's identity colour, or
// nothing when the ladder could not fit a rung.
//
// The banner is offered the width of the header and a height budget that leaves
// the terminal room for the composer, so a tall wordmark on a short terminal
// degrades to a shorter rung rather than to a broken layout. The art is padded
// to a common width so the rows form a rectangle; a wordmark whose rows have
// different lengths reads as a rendering fault even when every row is correct.
func (m model) renderBanner() []string {
	if m.cfg.Banner.Tool == "" && len(m.cfg.Banner.Art) == 0 {
		return nil
	}
	// Leave room for the header band itself plus the composer's rows and a blank
	// row, so a banner never squeezes the input off screen.
	budget := m.height - m.bannerReservedRows()
	art := RenderBanner(m.cfg.Banner, m.width, budget)
	if art.IsDropped() {
		return nil
	}
	// Rows are padded to the art's own width, not to the terminal's: the wordmark
	// is a rectangle, and the caller is what places it in the terminal's width.
	rows := make([]string, 0, len(art.ASCII))
	for _, line := range art.ASCII {
		rows = append(rows, padTo(m.theme.Title.Render(clampLine(line, art.Width)), art.Width))
	}
	return rows
}

// bannerReservedRows is the number of rows the header cannot give to the banner:
// the band itself and the composer's furniture. The transcript's share is
// whatever is left.
func (m model) bannerReservedRows() int {
	const composerRows = 4
	rows := 1 + composerRows
	if len(m.blocks) > 0 {
		rows++ // the strip between the transcript and the composer
	}
	return rows
}

// bannerReservedRowsFor is bannerReservedRows with the header's own height
// substituted, for the call that happens before the banner has been rendered.
// It is the only place the header is assumed to be one row, and it is an
// assumption rather than a measurement because the banner's height is what is
// being decided. The banner is measured against the space left over, so being one
// row optimistic here costs the banner a row of height, never the composer its
// input.

// bannerWithStatus draws the banner rows with the status pill on the first one,
// right-aligned, so both the identity and the session's state are on the same
// row rather than in different corners.
func (m model) bannerWithStatus(art []string, right string) string {
	// The status sits beside the art only when the art fits whole with a column
	// left over. Padding the art to the terminal's width and then appending the
	// status is what produced a row twice the terminal's width; truncating the art
	// to make room instead produced a cut-off wordmark with the pill jammed
	// against its last glyph, which reads worse than either.
	//
	// The same question is asked by bannerHeight, which has to know the answer to
	// charge the right number of rows to chrome. Asking it here too would be a
	// second place for the answer to change.
	beside := m.statusBesideBanner(art)

	var b strings.Builder
	for i, line := range art {
		switch {
		case i == 0 && beside:
			// Pad to the space the status leaves, then add it.
			room := m.width - lipgloss.Width(right)
			b.WriteString(padTo(line, room))
			b.WriteString(right)
		default:
			b.WriteString(padTo(line, m.width))
		}
		b.WriteString("\n")
	}
	if !beside {
		// The status goes below rather than being dropped: it is the one thing in
		// the header that changes without the operator doing anything, so it must
		// not be the thing that gets clipped.
		b.WriteString(m.band(padTo(right, m.width)))
		b.WriteString("\n")
	}
	// The last newline is removed because View adds one of its own after the
	// header. Left in place it made the header one row taller than the height
	// bannerHeight reported, so chromeHeight under-charged the header, the
	// viewport was given rows that were never there, and View wrote one row more
	// than the terminal has. The terminal scrolls, and the row that goes off the
	// bottom is the one the operator is typing into.
	return strings.TrimSuffix(b.String(), "\n")
}

// band draws one full-width strip of fixed furniture.
//
// The header is separated from the transcript by its own background rather than
// by a rule drawn beneath it. A rule under a row of text reads as a caption
// above a divider and costs a row to say what a filled bar says for free: the
// bar is unmistakably chrome, and it is the same row the identity occupies.
func (m model) band(content string) string {
	padded := padTo(clampLine(content, m.width), m.width)
	if !m.theme.Color {
		return padded
	}
	return lipgloss.NewStyle().
		Background(paletteColor(m.theme.Palette.Surface)).
		Render(padded)
}

// statusPill renders the session's state as a symbol-plus-word pill, the way a
// professional interface writes a status: the symbol and its text always travel
// together, so neither colour nor a single glyph has to carry the meaning alone.
//
// The state is a filled field, not coloured text. A word in Danger on the
// transcript's own ground is a word; the same word on a red field is an
// unmistakable event, and the header is where the operator looks when they want
// to know what the session is doing without reading the transcript to find out.
func (m model) statusPill() string {
	switch {
	case m.running:
		return m.theme.Pill.Render(spinnerFrames[m.spinner] + " RUNNING · " + duration(time.Since(m.started)))
	case m.state == stateCancelled:
		return m.theme.PillMuted.Render("■ CANCELLED")
	case m.state == stateFailed:
		return m.theme.PillCritical.Render("✕ FAILED")
	case m.following == false:
		// The user has scrolled back; say so, because new output is arriving
		// below the fold and silently not being seen is disorienting. Muted
		// rather than filled: nothing is wrong, the operator is just reading.
		return m.theme.PillMuted.Render("⤓ PAUSED")
	default:
		return m.theme.Pill.Render("● READY")
	}
}

// renderModeBar is the one-row strip that says where the keyboard is.
//
// The composer's hint text already describes the mode, but a hint is only
// legible if the operator reads it, and the mode is the first thing they need to
// know after a keystroke moved it. Carrying the mode in its own field means the
// answer to "where am I typing" is available at a glance and does not depend on
// parsing a sentence.
//
// The bar is drawn on the composer's ground, not the terminal's, so it reads as
// part of the input rather than as another rule.
func (m model) renderModeBar() string {
	label, style := "COMMAND", m.theme.ModeFg
	switch {
	case m.form != nil:
		label, style = "FILL IN", m.theme.ModeFg
	case m.regionFocus:
		label, style = "EXECUTIONS", m.theme.Selection
	}
	bar := " " + style.Render(label)
	if w := lipgloss.Width(bar); w < m.width {
		bar += strings.Repeat(" ", m.width-w)
	}
	// The mode bar carries its own fill, and the label inside it ends with a
	// reset that would clear that fill for every cell to the right of the label.
	// It is grounded twice, exactly as paintFrame grounds the frame: Inset goes
	// down first so the row still sits on the frame's field, and the mode bar's
	// own fill is raised on top of it and re-established after each reset. A
	// single bare Render would leave the strip past the label on the frame
	// colour instead of on the bar.
	return ground(m.theme.Inset, ground(m.theme.ModeBar, clampLine(bar, m.width), m.width), m.width)
}

// renderStrip is the row between the transcript and the composer. It is the
// session's pulse: silent while the operator follows the newest output, and
// once they scroll back it reports how much has appeared below the fold. The
// row is reserved whenever an execution exists, so the viewport does not
// resize as follow mode toggles.
