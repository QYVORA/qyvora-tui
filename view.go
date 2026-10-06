package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// tree glyphs, matching the shape a reader expects from a file tree.
//
// These are used only where there is real nesting to show, which after the
// redesign is the event log. Findings and artifacts are flat lists, and drawing
// branches for a list tells the reader there is a hierarchy that is not there.
const (
	treeBranch = "├─"
	treeLeaf   = "└─"
)

// gutterRail is the vertical rule that runs down the left of a command's body.
const gutterRail = "│"

// The columns a finding list is built from.
//
// A finding is a severity and a target with a sentence between them, and the
// sentence is what explains the other two. The severity is therefore given the
// width of its longest real word ("CRITICAL") and the target a bounded share of
// what is left, so every target in a run starts in the same column and the
// titles -- the part carrying the meaning -- get the width that remains.
//
// Nothing here is fixed in absolute terminal columns. The title column grows
// with the terminal instead, because a 34-character title cap left most of a
// wide terminal empty and truncated titles mid-word while doing it.
const (
	// severityColWidth is the width of the severity column, set to the longest
	// canonical severity word.
	severityColWidth = 8

	// severityRail is the coloured rule before each finding. It carries the
	// severity to the eye before the word is read, and it is a solid block
	// rather than a glyph so it still reads as a rail with colour switched off.
	severityRail = "▌"

	// minFindingTitle keeps a title readable when the terminal is narrow. Below
	// this the target is given up before the title is squeezed further.
	minFindingTitle = 18

	// The bounds on the target column. A target is an address, so it is worth
	// reserving room for it, but it is not worth more than a third of the line:
	// a very long address truncated to a share is no more useful than one
	// truncated to a column.
	minFindingTarget = 12
	maxFindingTarget = 30
)

// View renders the whole interface: a compact header, the scrolling session,
// and the composer pinned to the bottom.
//
// The terminal session is the workspace. There is no panel grid and no
// permanent dashboard: the middle of the screen is history, and the only fixed
// furniture is the prompt the user types into.
func (m model) View() string {
	if m.quitting {
		return ""
	}
	if m.width <= 0 || m.height <= 0 {
		// Before the first resize there is no geometry to lay out against;
		// render the composer alone rather than guessing at widths.
		return m.renderComposer()
	}

	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n")

	// The viewport takes everything between the header band and the composer.
	// It scrolls independently, so history stays reachable while a command runs.
	//
	// Regions are drawn only when the layout granted them width *and* there is
	// something to put in them. A region with no content is not rendered at
	// all: an empty column beside a working session is worse than the session
	// being wider.
	// The layout is resolved here, from the current state, rather than read
	// from whatever the last resize decided. State can change between a resize
	// and a render -- a run starting, the operator pressing F1 -- and resolving
	// here is what keeps the region widths and the drawn regions from
	// disagreeing.
	//
	// This is the pure half of applyLayout. View is a value method, so anything
	// done here is discarded by the time the program's model carries on: the
	// rebox below reflows a copy of the viewport and cannot disturb the real
	// widget or the scroll position the operator set.
	m.layout = m.resolveLayout(m.width)

	// The viewport may still sit at the dimensions of an earlier layout: a
	// block was appended, a region toggled, the terminal resized. Reflow it on
	// this copy so the mid column is exactly as wide as the layout granted the
	// transcript -- otherwise composeRegions trims lines back onto "…".
	m = m.rebox()

	// Both regions share the transcript's row budget. A panel drawn taller than
	// the transcript beside it has its last rows below the terminal, where they
	// are never seen: the newest run, which is the one the operator is watching,
	// is exactly the row that disappears.
	rows := m.viewport.Height
	left := m.capabilitiesRegion(m.layout.Navigation)
	right := m.executionsRegion(m.layout.Activity)
	if left != nil {
		left.Height = rows
	}
	if right != nil {
		right.Height = rows
	}

	// The widths come from the layout, and the regions are rendered at exactly
	// the widths it granted them, so the columns tile the terminal.
	// A boxed region is two columns wider than the layout granted it: the rule
	// and the space beside it. The separator between columns is the composer's,
	// so it is not counted here. Both numbers are the ones layout.go charged the
	// transcript for, which is what keeps the two from disagreeing.
	//
	// A width is taken from the rendered text rather than from the region
	// pointer, because a region can exist and still draw nothing: too short to
	// render, or empty. Charging the transcript for a panel that is not on screen
	// is how the columns end up over-running the terminal.
	// The transcript's row count is recorded as it is drawn. paintFrame needs to
	// know which rows are the transcript to put them on the Base field, and
	// counting rows by arithmetic from the frame's edges would be a second
	// version of chromeHeight that can disagree with the first.
	transcriptRows := m.viewport.Height

	leftBox, rightBox := boxedRegion(left, m.theme, -1), boxedRegion(right, m.theme, 1)
	navW, actW := 0, 0
	if leftBox != "" {
		navW = m.layout.Navigation + regionChrome
	}
	if rightBox != "" {
		actW = m.layout.Activity + regionChrome
	}
	switch {
	case navW > 0 && actW > 0:
		b.WriteString(composeRegions(m.viewport.View(), m.layout.Transcript,
			leftBox, navW, rightBox, actW, m.width))
	case navW > 0:
		b.WriteString(composeRegions(m.viewport.View(), m.layout.Transcript,
			leftBox, navW, "", 0, m.width))
	case actW > 0:
		b.WriteString(composeRegions(m.viewport.View(), m.layout.Transcript,
			"", 0, rightBox, actW, m.width))
	default:
		b.WriteString(m.viewport.View())
	}
	b.WriteString("\n")

	// The row between the transcript and the composer is the session's pulse:
	// while the operator is following the newest output it is silent, and the
	// moment they scroll back it reports how much has appeared below the fold.
	if len(m.blocks) > 0 {
		b.WriteString(m.renderStrip())
		b.WriteString("\n")
	}

	if m.form != nil {
		// The form replaces the composer while it is open. Two input lines at
		// once would leave the operator unsure which one their keystrokes are
		// going to.
		b.WriteString(m.renderForm())
		return m.paintFrame(strings.Split(b.String(), "\n"), transcriptRows)
	}
	b.WriteString(m.renderComposer())
	return m.paintFrame(strings.Split(b.String(), "\n"), transcriptRows)
}

// paintFrame puts the whole frame on the tool's own ground.
//
// The palette is a ladder of four fills, and a ladder only means anything if the
// steps are used in order: Inset is the field behind everything, and each lighter
// step is a surface raised off it. Without this the chrome is painted and the
// transcript is left on whatever background the terminal happens to have, so
// Surface has nothing to be raised above and the interface reads as a set of
// floating cards rather than one surface with panels on it.
//
// Painting is done here rather than at each call site because the ground has to
// reach the parts nothing draws: the empty rows above a short transcript, the
// columns between the panels, the row of the frame that holds no content at all.
// Those are the majority of the pixels on an idle screen, and they are exactly the
// ones that would otherwise show through as the terminal's own colour.
//
// The ground goes down first and the panels paint over it, so a row is painted
// twice but never twice in the wrong order: Inset covers the frame, and the
// transcript and panels that already carry their own fills replace it.
func (m model) paintFrame(rows []string, transcriptRows int) string {
	if !m.theme.Color {
		return strings.Join(rows, "\n")
	}
	painted := make([]string, 0, len(rows))
	for i, row := range rows {
		row = ground(m.theme.Inset, row, m.width)
		// The transcript is the field the work happens on: one step above the
		// frame, so the panels beside it read as raised off it. It starts below
		// the header and runs for the rows the viewport was boxed to.
		if i > 0 && i <= transcriptRows {
			row = ground(m.theme.Base, row, m.width)
		}
		painted = append(painted, row)
	}
	return strings.Join(painted, "\n")
}

// renderForm draws the open form's fields.
//
// Only the field being edited carries a cursor. Every field shows its value
// plainly, so a form is readable at a glance and does not need to be in edit
// mode to be reviewed.
func (m model) renderForm() string {
	f := m.form
	if f == nil {
		return m.renderComposer()
	}
	var b strings.Builder
	b.WriteString(m.theme.Rule.Render(strings.Repeat("─", max(1, m.width))))
	b.WriteString("\n")
	// The form takes the composer's place, so it takes the mode bar too. Without
	// it the keyboard's location is stated only in the footer hint, which is the
	// one part of the form that scrolls out of the way.
	b.WriteString(m.renderModeBar())
	b.WriteString("\n")
	title := m.theme.Title.Render("FILL IN  " + f.Capability.Name)
	b.WriteString(clampLine(title, m.width))
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
		row = padTo(clampLine(row, m.width), m.width)
		if i == f.focus {
			row = m.theme.Surface.Render(row)
		}
		b.WriteString(row)
		b.WriteString("\n")
	}

	if f.Err() != "" {
		b.WriteString(clampLine(m.theme.Failed.Render("  "+f.Err()), m.width))
		b.WriteString("\n")
	}
	// The keys are drawn with the shared key hint, so the form's footer and the
	// rest of the interface describe keys the same way.
	hint := "  " + keyHint(m.theme, "enter", "run", m.width-2) +
		"  " + keyHint(m.theme, "tab", "next field", m.width-2) +
		"  " + keyHint(m.theme, "esc", "cancel", m.width-2)
	b.WriteString(clampLine(hint, m.width))
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
func (m model) bannerHeight() int {
	return len(m.renderBanner())
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

// bannerWithStatus draws the banner rows with the status pill on the first one,
// right-aligned, so both the identity and the session's state are on the same
// row rather than in different corners.
func (m model) bannerWithStatus(art []string, right string) string {
	artW := 0
	for _, l := range art {
		if n := lipgloss.Width(l); n > artW {
			artW = n
		}
	}
	// The status sits beside the art only when the art fits whole with a column
	// left over. Padding the art to the terminal's width and then appending the
	// status is what produced a row twice the terminal's width; truncating the art
	// to make room instead produced a cut-off wordmark with the pill jammed
	// against its last glyph, which reads worse than either.
	beside := artW+1+lipgloss.Width(right) <= m.width

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
	return b.String()
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
	return m.theme.ModeBar.Render(clampLine(bar, m.width))
}

// renderStrip is the row between the transcript and the composer. It is the
// session's pulse: silent while the operator follows the newest output, and
// once they scroll back it reports how much has appeared below the fold. The
// row is reserved whenever an execution exists, so the viewport does not
// resize as follow mode toggles.
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
	b.WriteString(m.theme.Rule.Render(strings.Repeat("─", max(1, m.width))))
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
	line = padTo(clampLine(line, m.width), m.width)
	b.WriteString(m.theme.Raised.Render(line))
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
func (m model) transcriptLines() []string {
	if len(m.blocks) == 0 {
		// A session with nothing in it explains itself rather than looking
		// like a rendering failure. It is two lines that take no shelf space:
		// the composer is the prompt, this is the cue.
		if len(m.notices) == 0 {
			return []string{
				m.theme.Detail.Render("  ready for a command."),
				m.theme.Detail.Render("  help for keys · F1 the registry · F2 executions"),
			}
		}
	}
	var lines []string
	for i, b := range m.blocks {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, m.renderBlock(b)...)
	}
	if len(m.notices) > 0 {
		lines = append(lines, "")
		lines = append(lines, m.notices...)
	}
	return lines
}

// renderBlock renders one command and its execution as a session entry.
func (m model) renderBlock(b *block) []string {
	lines := m.renderCommand(b)
	return append(lines, m.gutter(m.renderBlockBody(b))...)
}

// gutter runs a faint vertical rule down the left of a command's body.
//
// Without it a transcript is just a list of lines, and the reader has to
// remember which command each result belongs to -- which is exactly what is
// hard after a few runs. With it, a command's outcome stays visibly attached to
// the command that produced it, however long the output between them gets.
//
// The rule is one step above the background rather than a full border, so it
// groups without boxing. Indentation beneath it is preserved: the body's own
// leading spaces are replaced by the gutter, not added to, so a finding and the
// events under it keep their relative nesting.
func (m model) gutter(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.TrimSpace(stripANSI(l)) == "" {
			out = append(out, l)
			continue
		}
		out = append(out, " "+m.theme.Gutter.Render(gutterRail)+" "+strings.TrimPrefix(l, "  "))
	}
	return out
}

// renderCommand echoes the command the way a terminal agent does: the user sees
// what they ran, so the history reads as a transcript of their own session
// rather than a log.
func (m model) renderCommand(b *block) []string {
	out := m.theme.Prompt.Render(promptGlyph) + m.renderCommandText(b.command, m.contentWidth())
	// No spacer row after the echo. The gutter that opens the block's body is
	// what separates the command from its results, and a blank line as well was
	// a row spent saying the same thing.
	return []string{m.theme.Surface.Render(padTo(clampLine(out, m.viewport.Width), m.viewport.Width))}
}

// renderCommandText highlights the command word and leaves the arguments
// readable, so a glance identifies what was run.
func (m model) renderCommandText(command string, width int) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(m.theme.Command.Render(fields[0]))
	for _, f := range fields[1:] {
		b.WriteString(" ")
		b.WriteString(m.theme.Arg.Render(f))
	}
	return clampLine(b.String(), max(8, width))
}

// renderBlockBody renders the state of an execution beneath its command.
func (m model) renderBlockBody(b *block) []string {
	var lines []string

	// A running command leads with what it is doing right now, and then shows
	// what the tool has actually printed, as it printed it. A scan that talks
	// as it works must read like a scan on a real terminal; hiding the tool's
	// own lines until it finishes makes a long run look dead. A running block
	// is never collapsed: hiding live output while it is live is how a long run
	// looks frozen.
	if b.status == StatusRunning {
		lines = append(lines, m.renderLive(b))
		lines = append(lines, m.renderProgress(b)...)
		lines = append(lines, m.renderOutput(b, true)...)
		return lines
	}

	lines = append(lines, m.renderSummary(b))
	lines = append(lines, m.renderGroups(b)...)
	// A finished run's own output is collapsed to a preview line by default.
	// The transcript is a session's history rather than a concatenated log, and
	// a 40-line banner printed by every command is what makes history
	// unreadable; the full text is one expand away, and the preview names its
	// size so nothing is silently hidden.
	lines = append(lines, m.renderOutput(b, b.expanded)...)
	lines = append(lines, m.renderEvents(b)...)
	return lines
}

// renderLive is the one-line status of a running execution.
func (m model) renderLive(b *block) string {
	what := "running"
	if b.progress.Message != "" {
		what = b.progress.Message
	} else if len(b.rows) > 0 {
		// The most recent event is the best available description of what the
		// tool is doing right now. The event's own detail is preferred over its
		// label: the label is the topic the tool emitted ("scan.progress"), while
		// the detail is the sentence it wrote for a person ("scanning cell 812447
		// of 1000000"). Falling back to the label leaves the operator reading a
		// topic name where a description was available.
		last := b.rows[len(b.rows)-1]
		what = last.Detail
		if what == "" {
			what = last.Label
		}
	}
	return "  " + m.theme.Running.Render(spinnerFrames[m.spinner]) + " " +
		m.theme.Value.Render(truncate(what, max(16, m.contentWidth()-8)))
}

// renderProgress draws a bar only when the tool reported a percentage. An
// invented bar would be a guess about work the tool never reported.
//
// When the tool reported counts as well as a percentage, they are drawn beside
// the bar. A percentage alone answers "how far along", which is the less useful
// half: on a scan of two million cells "81%" says nothing about whether the run
// is nearly over or has an hour left. The counts say both, and the tool
// published them, so throwing them away to save a few columns is a poor trade.
//
// The counts are grouped rather than separated. "812447 / 1000000" is read as
// one number over another; "812447 of 1000000" costs five more columns to say
// the same thing.
func (m model) renderProgress(b *block) []string {
	if !b.progress.HasBar {
		return nil
	}
	p := b.progress
	pct := clampPercent(p.Percent)

	counts := ""
	if p.Total > 0 && p.Current >= 0 {
		counts = m.theme.Detail.Render(fmt.Sprintf("  %s / %s", commas(p.Current), commas(p.Total)))
	}

	// The bar takes what is left after the indent, the counts and the percentage.
	// Measuring the finished line and then clamping it is how the count used to
	// be cut off at narrow widths instead of the bar giving way.
	const pctW = 5
	fixed := 2 + countsLen(counts) + pctW
	width := m.contentWidth() - fixed
	if width > 24 {
		width = 24
	}
	if width < minBarCells {
		// Too narrow for a bar and its numbers together. The numbers are the more
		// precise statement of the same fact, so they win and the bar is dropped
		// rather than rendering both at a width where neither can be read.
		//
		// The counts are abbreviated before they are given up entirely, because
		// "812k / 1.0M" still carries the magnitude and "81%" does not. Only when
		// even the abbreviation will not fit does the percentage stand alone.
		pctText := m.theme.Detail.Render(fmt.Sprintf("%3.0f%%", pct))
		for _, pair := range [][2]string{
			{shortCount(p.Current), shortCount(p.Total)},
			{shortCount(p.Current), commas(p.Total)},
			{commas(p.Current), commas(p.Total)},
		} {
			if p.Total <= 0 || p.Current < 0 {
				break
			}
			row := m.theme.Detail.Render(fmt.Sprintf("  %s / %s", pair[0], pair[1])) + " " + pctText
			if lipgloss.Width(stripANSI(row)) <= m.contentWidth() {
				return []string{row}
			}
		}
		return []string{clampLine("  "+pctText, m.contentWidth())}
	}

	filled := int(pct / 100 * float64(width))
	bar := m.theme.BarFill.Render(strings.Repeat("█", filled)) +
		m.theme.BarEmpty.Render(strings.Repeat("░", max(0, width-filled)))
	return []string{"  " + bar + " " + m.theme.Detail.Render(fmt.Sprintf("%3.0f%%", pct)) + counts}
}

// countsLen is the width of a rendered counts string.
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
func (m model) renderSummary(b *block) string {
	mark := m.theme.Success.Render("✓")
	verb := "completed"
	var style lipgloss.Style = m.theme.Success

	switch b.status {
	case StatusFailed:
		mark = m.theme.Failed.Render("✗")
		verb = "failed"
		style = m.theme.Failed
	case StatusCancelled:
		mark = m.theme.Cancelled.Render("⊘")
		verb = "cancelled"
		style = m.theme.Cancelled
	}

	sep := m.theme.Detail.Render(" · ")
	// The lead is what the row is: the mark and the outcome. It is never dropped
	// or shortened, because a summary that cannot say whether the run passed is
	// not a summary.
	lead := style.Render(verb + " " + duration(b.elapsed()))

	// The tallies are ordered by how much they change what the operator does next.
	// Findings come before artifacts and events because a security tool is run for
	// what it found; an artifact count is background, and an event count is
	// diagnostic detail that is one keystroke away.
	var parts []string
	if b.err != "" {
		// A failed run should say why without the user having to expand it. The
		// reason is placed after the tallies for that reason: it is the longest
		// and least compressible part, and a run that failed with a 200-character
		// stack must not push "12 findings" off the end of the row.
		parts = append(parts, m.theme.Failed.Render(b.err))
	}
	if n := len(b.findings); n == 0 {
		// "No findings" is only good news from a run that actually finished its
		// work. On a failed run it is drawn dim: a scan that died before
		// looking is not a clean bill of health, and colouring it green says
		// otherwise.
		absent := m.theme.Detail
		if b.status == StatusDone {
			absent = m.theme.Success
		}
		parts = append(parts, absent.Render("no findings"))
	} else {
		parts = append(parts, m.theme.styleForSeverityWord(worstSeverity(b.findings)).
			Render(plural(n, "finding", "findings")))
	}
	if n := len(b.artifacts); n > 0 {
		parts = append(parts, m.theme.Value.Render(plural(n, "artifact", "artifacts")))
	}
	if n := b.known; n > 0 {
		parts = append(parts, m.theme.Detail.Render(plural(n, "event", "events")))
	}

	// Sort the trailing parts by how much they matter, most important first.
	// Fitting then drops from the end, so what survives is the finding tally
	// rather than whatever happened to be appended last.
	sortSummaryParts(parts)

	// The row is indent + mark + space + lead, and then each part costs a
	// separator plus its own text. The budget is what is left after the fixed
	// part; the loop spends it one part at a time.
	//
	// The previous version built the parts, joined them, and returned the result,
	// which overran the terminal by whatever the parts needed beyond it. The
	// viewport then cut the row at its own width, which is how a finished run came
	// to read "12 finding" with the artifact and event counts gone entirely.
	const minPartWidth = 8
	sepW := lipgloss.Width(sep)
	var out strings.Builder
	out.WriteString("  ")
	out.WriteString(mark)
	out.WriteString(" ")
	out.WriteString(lead)
	room := m.contentWidth() - 3 - lipgloss.Width(mark) - lipgloss.Width(lead)
	for _, part := range parts {
		if room < sepW+minPartWidth {
			break
		}
		out.WriteString(sep)
		out.WriteString(truncate(stripANSI(part), room-sepW))
		room -= sepW + lipgloss.Width(stripANSI(part))
	}
	return clampLine(out.String(), m.contentWidth())
}

// summaryPartWeight ranks a trailing summary part by how much it matters. Lower
// sorts first, so the parts that get dropped when the row is too narrow are the
// least informative ones.
func summaryPartWeight(text string) int {
	switch {
	case strings.Contains(text, "finding"):
		return 0
	case strings.Contains(text, "no findings"):
		return 0
	case strings.Contains(text, "artifact"):
		return 1
	case strings.Contains(text, "event"):
		return 2
	default:
		// A failure reason, or anything a tool printed that is not a tally.
		return 3
	}
}

func sortSummaryParts(parts []string) {
	sort.SliceStable(parts, func(i, j int) bool {
		return summaryPartWeight(parts[i]) < summaryPartWeight(parts[j])
	})
}

// renderGroups renders the findings and artifacts of a finished execution.
//
// Findings are grouped because "what did it find" is the question a security
// tool is run to answer, and the answer is scannable only if the severity and
// the target line up from row to row.
func (m model) renderGroups(b *block) []string {
	var lines []string

	if n := len(b.findings); n > 0 {
		fs := make([]Finding, len(b.findings))
		copy(fs, b.findings)
		sortFindings(fs)
		lines = append(lines, m.groupHeading("findings", n))
		titleW, targetW := m.findingColumns()
		for _, f := range fs {
			lines = append(lines, m.renderFinding(f, titleW, targetW))
		}
	}

	if n := len(b.artifacts); n > 0 {
		lines = append(lines, m.groupHeading("artifacts", n))
		sizeW := m.artifactSizeWidth()
		for _, a := range b.artifacts {
			lines = append(lines, m.renderArtifact(a, sizeW))
		}
	}
	return lines
}

// groupHeading labels a section of a finished run.
//
// The label is set in the structure colour rather than the text colour and the
// count beside it is dimmer again, which is what separates "here is a section"
// from "here is a result" without needing a box or a blank line above it.
func (m model) groupHeading(label string, count int) string {
	h := "  " + m.theme.Heading.Render(strings.ToUpper(label))
	if count > 0 {
		h += "  " + m.theme.Count.Render(strconv.Itoa(count))
	}
	return h
}

// findingColumns divides the transcript width between the finding title and the
// target.
//
// The title takes everything the target does not need. Titles are prose and
// targets are addresses, so when space runs short the address is the part that
// should be cut, and it is cut to a bounded column rather than to whatever is
// left over.
func (m model) findingColumns() (title, target int) {
	// indent + rail + space + severity + gap + gap
	const fixed = 2 + 1 + 1 + severityColWidth + 2 + 2
	avail := m.contentWidth() - fixed
	if avail < minFindingTitle+minFindingTarget {
		// Too narrow for both. Below about eight columns an address is not
		// readable at all, so the target is dropped and its space goes to the
		// title: an address cut to four characters identifies nothing, and
		// saying nothing is better than saying something false.
		if avail-minFindingTitle < 8 {
			return max(minFindingTitle, avail), 0
		}
		return minFindingTitle, avail - minFindingTitle
	}
	target = clampInt(avail/3, minFindingTarget, maxFindingTarget)
	title = max(minFindingTitle, avail-target-1)
	return title, target
}

// artifactSizeWidth is the column artifact sizes share, so a list of files
// lines up by size the way findings line up by target.
func (m model) artifactSizeWidth() int {
	const fixed = 2 + 1 + 1 + 1
	return clampInt(m.contentWidth()-fixed, 0, 12)
}

// renderFinding draws one finding: rail, severity, title, target.
//
// The rail is what makes a long list scannable. Colour alone would put the
// severity last, after the eye has already read the sentence; a solid block at a
// fixed offset puts the worst findings at a glance and survives NO_COLOR, where
// the word is carrying the whole meaning on its own.
func (m model) renderFinding(f Finding, titleW, targetW int) string {
	style := m.theme.styleForSeverityWord(severityWord(f.Severity))
	line := "  " + style.Render(severityRail) + " " +
		style.Render(m.theme.severityTag(f.Severity)) + "  " +
		m.theme.Value.Render(pad(truncate(f.Title, titleW), titleW))
	if targetW > 0 && f.Target != "" {
		line += " " + m.theme.Column.Render(pad(truncate(f.Target, targetW), targetW))
	}
	return clampLine(line, m.contentWidth())
}

// renderArtifact draws one artifact: name, then size in its own column.
//
// The name is truncated rather than the size, because a size is a short exact
// value and a truncated one is a lie, while a truncated path is still
// recognisable.
func (m model) renderArtifact(a Artifact, sizeW int) string {
	line := "  " + m.theme.Detail.Render("▪") + " " + m.theme.Value.Render(a.Name)
	if a.Size != "" && sizeW > 0 {
		gap := m.contentWidth() - 4 - lipgloss.Width(a.Name) - sizeW
		if gap < 1 {
			gap = 1
		}
		line += strings.Repeat(" ", gap) + m.theme.Column.Render(pad(a.Size, sizeW))
	}
	return clampLine(line, m.contentWidth())
}

// renderEvents renders the optional raw event log.
//
// The event stream is kept in full and can be inspected, but it is hidden by
// default: a wall of JSON is the thing an agent terminal exists to avoid, and
// it is one keystroke away when it is actually wanted.
func (m model) renderEvents(b *block) []string {
	if !m.showEvents || len(b.rows) == 0 {
		return nil
	}
	head := "  " + m.theme.Heading.Render("EVENTS") + "  " + m.theme.Count.Render(strconv.Itoa(len(b.rows)))
	if b.rowsOmitted {
		head += " " + m.theme.Detail.Render("(tail kept)")
	}
	lines := []string{head}
	for i, row := range b.rows {
		branch := treeBranch
		if i == len(b.rows)-1 {
			branch = treeLeaf
		}
		lines = append(lines, m.renderRow(row, branch))
		for _, k := range sortedKeys(row.Data) {
			lines = append(lines, "      "+m.theme.Detail.Render(pad(k, 18))+
				m.theme.Detail.Render(truncate(dataString(row.Data, k), max(16, m.contentWidth()-30))))
		}
	}
	return lines
}

// renderOutput shows what the command printed, as written.
//
// It is wrapped rather than clipped, because clipping throws away the far end
// of a table, which is the part holding the answer to the question the operator
// asked. Wrapping keeps every column reachable and is what makes a wide
// capability list readable in a narrow terminal.
//
// A finished run's output collapses to a one-line preview unless expanded: the
// transcript reads as history, and the shape of a tool's banner is not a
// session's content. The preview names the size -- and says when the tail is
// being shown, so bounding a runaway command never hides its truncation.
func (m model) renderOutput(b *block, expanded bool) []string {
	if len(b.output) == 0 {
		return nil
	}
	if !expanded {
		note := plural(len(b.output), "line", "lines")
		if b.outputOmitted {
			note += " shown (tail kept)"
		}
		// An affordance, not a section. It is dimmed and set further in than a
		// heading so it reads as a note attached to the run rather than as the
		// first line of its output.
		//
		// The keys it names are the ones that actually work from where the
		// operator is. Expanding is done from the executions region, so F2 on
		// its own only moves focus there; saying "F2 expand" sent people
		// pressing a key that visibly did nothing. With the region already
		// focused the step is not needed and is not named.
		keys := "· F2 focus, enter expand"
		if m.regionFocus {
			keys = "· enter expand"
		}
		return []string{"   " + m.theme.Detail.Render("output "+note) + " " +
			m.theme.Hint.Render(keys)}
	}
	// A printed block is quoted under a rule rather than mixed into the tree:
	// the tree is for results the interface understood, and this is the tool
	// speaking for itself.
	lines := []string{"  " + m.theme.Heading.Render("OUTPUT")}
	if b.outputOmitted {
		lines = append(lines, "   "+m.theme.Detail.Render("(truncated to the last "+plural(outputCap, "line", "lines")+")"))
	}
	width := m.contentWidth() - 2
	for _, raw := range b.output {
		// Wrap first, highlight second. Highlighting before wrapping would put
		// escape sequences into the string the wrapper measures, and it would cut
		// one in half at the break point -- the wrapped line then paints the rest
		// of the terminal the colour it was meant to end in.
		for _, line := range wrapText(raw, width) {
			lines = append(lines, "  "+highlightLine(line, m.theme))
		}
	}
	return lines
}

// wrapText breaks a line at width, preferring spaces so words stay intact.
//
// The spaces inside the line are content, not separators. A run of them is the
// gap between two columns, and collapsing it -- which is what a
// strings.Fields-based wrap does -- turns an aligned table into prose and a
// tree drawing into a staircase. So the runs are carried through verbatim and a
// line that already fits is returned untouched.
//
// Wrapping is the one place where exactness is impossible: a line too wide for
// the terminal has to become several. Where it breaks, it breaks at a space, and
// the spaces at the break are consumed rather than carried to the next line. A
// continuation does not inherit the original indentation, because the caller has
// already indented every line it draws and a second indent would be a level of
// depth the source never had.
//
// A word longer than the line is split by hand rather than dropped. Losing
// content silently is worse than an awkward break.
func wrapText(s string, width int) []string {
	s = strings.ReplaceAll(strings.TrimRight(s, "\r"), "\t", "    ")
	if width < 8 {
		return []string{s}
	}
	if lipgloss.Width(s) <= width {
		return []string{s}
	}

	// Width is display width, not rune count: a line of CJK text or an emoji is
	// wider than its length, and measuring runes lets it overflow the column.
	var b lineBuilder
	b.limit = width
	b.runes = make([]rune, 0, width)
	out := make([]string, 0, 4)
	// A line of nothing but spaces is not a line. It appears when the
	// indentation is wider than the width, and drawing it would push everything
	// else down the transcript for no information.
	flush := func() {
		line := strings.TrimRight(b.String(), " ")
		b.reset()
		if strings.TrimSpace(line) == "" {
			return
		}
		out = append(out, line)
	}

	// Indentation belongs to the line it starts on, and is kept for exactly that
	// long. Indentation past the width is dropped rather than wrapping into a
	// line of nothing but spaces.
	runes := []rune(s)
	i := 0
	for i < len(runes) && runes[i] == ' ' && b.fits(1) {
		b.add(runes[i])
		i++
	}

	// pending counts the spaces seen since the last word, so a word can be placed
	// together with the gap that introduces it -- which is what keeps two columns
	// two columns.
	pending := 0
	for i < len(runes) {
		if runes[i] == ' ' {
			pending++
			i++
			continue
		}
		start := i
		for i < len(runes) && runes[i] != ' ' {
			i++
		}
		word := runes[start:i]
		ww := lipgloss.Width(string(word))

		switch {
		case b.empty():
			// The first word on a line. Any spaces left over are interior
			// alignment from the original and are kept as far as they fit.
			b.addSpaces(pending)
			pending = 0
			b.addWord(&out, flush, word)
		case b.fits(pending + ww):
			b.addSpaces(pending)
			pending = 0
			b.addWord(&out, flush, word)
		default:
			// The word does not fit after the gap. Break here, and the gap is the
			// break: it goes rather than starting the next line with it.
			pending = 0
			flush()
			b.addWord(&out, flush, word)
		}
	}
	flush()
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

// lineBuilder accumulates one wrapped line, tracking its display width.
type lineBuilder struct {
	runes []rune
	width int
	limit int
}

func (b *lineBuilder) empty() bool { return len(b.runes) == 0 }

func (b *lineBuilder) fits(w int) bool { return b.width+w <= b.limit }

func (b *lineBuilder) add(r rune) {
	b.runes = append(b.runes, r)
	b.width += lipgloss.Width(string(r))
}

func (b *lineBuilder) addSpaces(n int) {
	for i := 0; i < n && b.fits(1); i++ {
		b.add(' ')
	}
}

func (b *lineBuilder) reset() {
	b.runes = b.runes[:0]
	b.width = 0
}

func (b *lineBuilder) String() string { return string(b.runes) }

// addWord appends a word, breaking the line as many times as the word needs.
//
// A word wider than the whole line cannot be made to fit by any amount of
// breaking, so it is split by width. It is never dropped.
func (b *lineBuilder) addWord(out *[]string, flush func(), word []rune) {
	for _, r := range word {
		w := lipgloss.Width(string(r))
		if !b.fits(w) && !b.empty() {
			flush()
		}
		// A single glyph wider than the line, such as a full-width CJK character
		// on an eight-column transcript, goes on anyway: clipping it would lose
		// it, and one overflowing glyph is better than none.
		if !b.fits(w) && w > b.limit {
			b.add(r)
			continue
		}
		b.add(r)
	}
}

func (m model) renderRow(row eventRow, branch string) string {
	level := strings.ToUpper(row.Level)
	style := m.theme.Detail
	switch level {
	case "ERROR", "FATAL":
		style = m.theme.Failed
	case "WARN", "WARNING":
		style = m.theme.Medium
	case "INFO":
		style = m.theme.Info
	}
	out := "  " + m.theme.Detail.Render(branch) + " " + style.Render(pad(level, 5)) +
		m.theme.Value.Render(pad(row.Label, 24))
	if row.Detail != "" {
		out += " " + m.theme.Detail.Render(truncate(row.Detail, max(16, m.contentWidth()-40)))
	}
	return clampLine(out, m.contentWidth())
}

// clampPercent keeps a reported percentage inside its natural bounds. A tool
// reporting 140% should not draw a bar that overflows its track.
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
func (m *model) applyLayout(width, height int) {
	m.layout = m.resolveLayout(width)
	tw := m.transcriptWidth(width)
	m.viewport = viewport.New(max(1, tw), m.viewportHeight(height))
	// The composer spans the whole terminal, not the transcript: it is the one
	// element that addresses the operator, and its text has no indentation to
	// reserve for.
	m.input.Width = max(8, width-lipgloss.Width(m.input.Prompt)-2)
}

// viewportHeight is how many rows the transcript owns between the header band
// and the composer.
func (m *model) viewportHeight(termHeight int) int {
	return max(1, termHeight-m.chromeHeight())
}

// chromeHeight is the number of rows the fixed furniture occupies, so the
// transcript can be given everything that is left.
//
// The header counts as one row because its rule is its border rather than a row
// of its own. The strip is counted whenever an execution exists, whether or not
// it currently has anything to say, so the viewport does not resize every time
// follow mode toggles: a viewport that changes height on a scroll moves the
// transcript under the fingers using it.
//
// An open form is measured rather than assumed. A form stands in for the
// composer but can be several rows taller than it, and charging it the
// composer's two rows is how a long form pushes its own fields off the top of
// the terminal.
func (m *model) chromeHeight() int {
	rows := m.headerHeight()
	if len(m.blocks) > 0 {
		rows++ // the strip between transcript and composer
	}
	if m.form != nil {
		return rows + m.formHeight()
	}
	return rows + 3 // the composer's rule, mode bar and input
}

// headerHeight is how many rows the header band occupies: one, or the height of
// the banner when the tool supplied one.
//
// The banner is charged to the transcript here, which is what keeps a tall
// wordmark from pushing the composer's input row off the bottom of a short
// terminal. Chrome is the difference between the terminal and the viewport, so
// anything drawn in the header has to be counted in it or the viewport is built
// taller than the space it was given.
func (m model) headerHeight() int {
	if h := m.bannerHeight(); h > 0 {
		return h
	}
	return 1
}

// formHeight is the number of rows an open form draws.
func (m model) formHeight() int {
	f := m.form
	if f == nil {
		return 0
	}
	rows := 3 + len(f.fields) // its rule, mode bar, title, one row per field
	if f.Err() != "" {
		rows++
	}
	return rows + 1 // the key hint footer
}

// rebox reflows the viewport at the dimensions the current layout grants, on a
// copy of the model. The scroll position the operator set is preserved:
// SetContent reflows the history at the new width and clamps YOffset to the
// new total, and following models stay pinned to the bottom.
func (m model) rebox() model {
	tw, th := m.transcriptWidth(m.width), m.viewportHeight(m.height)
	if m.viewport.Width == tw && m.viewport.Height == th {
		return m
	}
	m.viewport.Width = tw
	m.viewport.Height = th
	m.refreshViewport()
	return m
}

// capabilitiesRegion builds the capability region, or nil when there is nothing
// to put in it.
//
// Returning nil rather than an empty region is the point: the layout is told
// there is a navigation region only when one has content, so a tool with no
// registry never gets a column of nothing.
//
// Each capability takes two lines: the name, then its detail indented beneath
// it. The region is 16 to 24 columns wide, and a name like "Profile target
// baseline" uses all of it, so a single line cannot carry both the name and the
// facts that change a decision. Two lines can, and the alternative -- a row of
// truncated fragments -- is what makes a narrow column unreadable.
func (m model) capabilitiesRegion(width int) *Region {
	if width <= 0 || !m.showCapabilities || m.caps == nil || len(m.caps.Items) == 0 {
		return nil
	}
	r := NewRegion("CAPABILITIES", width)
	r.Empty = "none published"
	r.Height = m.capabilitiesBudget()
	return m.fillCapabilitiesRegion(r)
}

// capabilitiesBudget is how many rows the capability region may draw, title and
// rule included.
//
// The budget comes from viewportHeight rather than from a guess at the terminal
// height. The region is drawn beside the transcript and shares its rows, so that
// is the number that is actually true at draw time, and computing it from the
// same function the viewport uses is what keeps the two from disagreeing. The
// previous guess of height-10 meant a short terminal hid capabilities that would
// have fitted, and then reported a "+N more" count unrelated to how many were
// really hidden.
func (m model) capabilitiesBudget() int {
	h := m.viewportHeight(m.height)
	if h <= chromeRows {
		return 0
	}
	return h
}

// fillCapabilitiesRegion adds as many capabilities as the region's budget
// allows, complete ones only, and says how many were left out.
//
// A capability takes its name, then its detail indented beneath it, then its
// note. Rows that do not fit are not drawn half: a row cut off by the region's
// bottom edge is worse than one capability fewer, because the operator cannot
// tell an incomplete entry from a complete one.
func (m model) fillCapabilitiesRegion(r *Region) *Region {
	entries := m.caps.Entries()
	// chromeRows is subtracted because the title and the rule are part of the
	// budget and are drawn before any content.
	budget := r.Height - chromeRows
	if r.Height <= 0 {
		budget = 0
	}
	used, shown := 0, 0
	// When anything is left out, the last row is spent saying so. Reserving it up
	// front rather than adding it afterwards is what makes the notice reliable: a
	// loop that fills the budget and only then tries to append "+N more" has
	// already used the row, so the notice is the thing that gets dropped and the
	// count of hidden capabilities goes unmentioned.
	moreRow := 1
	for _, e := range entries {
		rows := capabilityRows(e)
		if budget > 0 && used+rows+moreRow > budget {
			break
		}
		marker, style := "●", m.theme.Value
		switch {
		case !e.Available:
			// An unavailable capability is dimmed and says why, so the region
			// explains the absence instead of looking like an oversight.
			marker, style = "○", m.theme.Detail
		case e.Note == "live provider":
			style = m.theme.Warning
		}
		// The style is applied here rather than only chosen: a dimmed row that
		// renders identically to a live one is not dimmed, and the distinction
		// the marker makes is the only thing left carrying it.
		r.Add(style.Render(clampLine(marker+" "+e.Name, r.Width-2)))
		if e.Detail != "" {
			r.Add(m.theme.Detail.Render(clampLine("  "+e.Detail, r.Width-2)))
		}
		if e.Note != "" && e.Note != "live provider" {
			r.Add(m.theme.Hint.Render(clampLine("  "+e.Note, r.Width-2)))
		}
		used += rows
		shown++
	}
	if remaining := len(entries) - shown; remaining > 0 {
		// The count is what was actually left out, which is only true now that
		// the loop stops on the budget rather than on a guess.
		r.Add(m.theme.Hint.Render(clampLine(fmt.Sprintf("  +%d more", remaining), r.Width-2)))
	}
	return r
}

// capabilityRows is how many rows one capability occupies: its name, its detail
// and its note, counting only the ones it actually has.
//
// Counting it rather than assuming two is what keeps the last capability from
// being drawn when only half of it fits.
func capabilityRows(e Entry) int {
	n := 1
	if e.Detail != "" {
		n++
	}
	if e.Note != "" && e.Note != "live provider" {
		n++
	}
	return n
}

// boxedRegion renders a region with its inner rule, or an empty string when
// there is nothing to draw.
//
// The empty string is the signal, not a special case: a caller that charges the
// transcript for a column has to know whether the panel is on screen, and a
// region that renders nothing is not on screen.
func boxedRegion(r *Region, t Theme, side int) string {
	if r == nil {
		return ""
	}
	return r.Boxed(t, side)
}

// executionsRegion builds the executions region: the session's navigator.
//
// It is the persistent right-hand column once any command has run: the running
// command at the top with its live counts, the finished runs beneath it, and
// each row expandable. It replaces the old single-purpose activity view,
// because a run finishing is exactly when its history becomes useful, and a
// column that vanished at that moment would be hiding the thing it had shown.
//
// A width of zero means the layout granted no region, and returning a region
// anyway would charge the transcript columns for a panel that draws nothing.
func (m model) executionsRegion(width int) *Region {
	if width <= 0 || len(m.blocks) == 0 {
		return nil
	}
	r := NewRegion("EXECUTIONS", width)
	r.Focused = m.regionFocus
	r.Empty = "no executions"
	entries := m.execEntries(width)
	if len(entries) == 0 {
		return r
	}
	rows, more := m.expandedRegionWindow(entries)
	for _, line := range rows {
		r.Add(line)
	}
	if more {
		r.Add(m.theme.Hint.Render(clampLine("  ↓ scroll for older", width)))
	}
	return r
}

// resolveLayout resolves the geometry for a width from the model's current state.
//
// It is pure: it reads the model and returns a Layout, changing nothing. Both
// the resize path and the render path use it, which is what stops the region
// widths and the drawn regions from being computed from two different pictures
// of the model.
//
// A region is offered only when it will actually be drawn: the tool has
// something to put in it, and the operator has asked for it. Offering a region
// that then renders nothing reserves a column for a blank, which is the outcome
// the whole rule exists to prevent.
func (m model) resolveLayout(width int) Layout {
	l := LayoutFor(width, m.height, LayoutOptions{
		Navigation: m.showCapabilities && m.caps != nil && len(m.caps.Items) > 0,
		// The executions region exists for the whole session after the first
		// command, not just while a run is in flight: a finished run's history
		// is exactly what the navigator is for. This is what keeps the region
		// from vanishing the moment a command completes.
		Activity: len(m.blocks) > 0,
	})
	// A region that turned out to have no content gives its width back, so the
	// transcript is not left narrower than it needs to be.
	if l.Navigation > 0 && m.capabilitiesRegion(l.Navigation) == nil {
		l = LayoutFor(width, m.height, LayoutOptions{Activity: len(m.blocks) > 0})
	}
	if l.Activity > 0 && m.executionsRegion(l.Activity) == nil {
		l = LayoutFor(width, m.height, LayoutOptions{
			Navigation: m.showCapabilities && m.caps != nil && len(m.caps.Items) > 0,
		})
	}
	return l
}

// transcriptWidth is the width the viewport is built at: the layout's share, or
// the whole terminal when no region is drawn.
func (m model) transcriptWidth(width int) int {
	if !m.layout.ShowRegions {
		return max(1, width)
	}
	return max(1, m.layout.Transcript)
}
