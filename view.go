package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// tree glyphs, matching the shape a reader expects from a file tree.
const (
	treeBranch = "├─"
	treeLeaf   = "└─"
	treePipe   = "│ "
	treeBlank  = "  "
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
	// A hairline rule under the header separates identity from content without
	// drawing a box around everything.
	b.WriteString(m.theme.Rule.Render(strings.Repeat("─", max(1, m.width))))
	b.WriteString("\n")

	// The viewport takes everything between the header rule and the composer.
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

	left := m.capabilitiesRegion(m.layout.Navigation)
	right := m.executionsRegion(m.layout.Activity)

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
		return b.String()
	}
	b.WriteString(m.renderComposer())
	return b.String()
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
	title := m.cfg.Title
	if title == "" {
		title = "QYVORA"
	}
	left := m.theme.Title.Render("▇ ") + m.theme.Title.Render(title)
	if m.cfg.Version != "" {
		left += "  " + m.theme.Version.Render(m.cfg.Version)
	}

	right := m.statusPill()
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		// Too narrow to sit side by side. The identity matters more than the
		// status, so the status gives way rather than both being clipped.
		return clampLine(left, m.width)
	}
	return m.theme.Surface.Render(padTo(clampLine(left+strings.Repeat(" ", gap)+right, m.width), m.width))
}

// statusPill renders the session's state as a symbol-plus-word pill, the way a
// professional interface writes a status: the symbol and its text always travel
// together, so neither colour nor a single glyph has to carry the meaning alone.
func (m model) statusPill() string {
	switch {
	case m.running:
		return m.theme.Running.Render(spinnerFrames[m.spinner] + " RUNNING · " + duration(time.Since(m.started)))
	case m.state == stateCancelled:
		return m.theme.Cancelled.Render("■ CANCELLED")
	case m.state == stateFailed:
		return m.theme.Failed.Render("✕ FAILED")
	case m.following == false:
		// The user has scrolled back; say so, because new output is arriving
		// below the fold and silently not being seen is disorienting.
		return m.theme.Hint.Render("⤓ PAUSED")
	default:
		return m.theme.Ready.Render("● ready")
	}
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
	// panel: a rule above it and a green prompt are enough.
	var b strings.Builder
	b.WriteString(m.theme.Rule.Render(strings.Repeat("─", max(1, m.width))))
	b.WriteString("\n")

	line := m.input.View()
	if hint := m.hint(); hint != "" {
		hint = m.theme.Hint.Render(hint)
		space := m.width - lipgloss.Width(line) - lipgloss.Width(hint)
		if space >= 2 {
			line += strings.Repeat(" ", space) + hint
		}
	}
	line = padTo(clampLine(line, m.width), m.width)
	b.WriteString(m.theme.Surface.Render(line))
	return b.String()
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
	return append(lines, m.renderBlockBody(b)...)
}

// renderCommand echoes the command the way a terminal agent does: the user sees
// what they ran, so the history reads as a transcript of their own session
// rather than a log.
func (m model) renderCommand(b *block) []string {
	out := m.theme.Prompt.Render("❯ ") + m.renderCommandText(b.command, m.contentWidth())
	return []string{m.theme.Surface.Render(padTo(clampLine(out, m.viewport.Width), m.viewport.Width)), "  "}
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
	lines = append(lines, m.renderResults(b))
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

// renderResults is the one-line tally of what a finished run produced: the
// findings, artifacts and events it left behind. It is the compact answer to
// "what did that do?" that a security operator asks of every command, and it
// stays visible whether or not the run's output is expanded.
func (m model) renderResults(b *block) string {
	counts := []string{}
	if n := len(b.findings); n > 0 {
		counts = append(counts, m.theme.Success.Render(plural(n, "finding", "findings")))
	} else {
		counts = append(counts, m.theme.Detail.Render("no findings"))
	}
	if n := len(b.artifacts); n > 0 {
		counts = append(counts, m.theme.Value.Render(plural(n, "artifact", "artifacts")))
	}
	if n := b.known; n > 0 {
		counts = append(counts, m.theme.Value.Render(plural(n, "event", "events")))
	}
	return "  " + strings.Join(counts, " · ")
}

// renderLive is the one-line status of a running execution.
func (m model) renderLive(b *block) string {
	what := "running"
	if b.progress.Message != "" {
		what = b.progress.Message
	} else if len(b.rows) > 0 {
		// The most recent event is the best available description of what the
		// tool is doing right now.
		last := b.rows[len(b.rows)-1]
		if last.Detail != "" {
			what = last.Label
		} else {
			what = last.Label
		}
	}
	return "  " + m.theme.Running.Render(spinnerFrames[m.spinner]) + " " +
		m.theme.Value.Render(truncate(what, max(16, m.contentWidth()-8)))
}

// renderProgress draws a bar only when the tool reported a percentage. An
// invented bar would be a guess about work the tool never reported.
func (m model) renderProgress(b *block) []string {
	if !b.progress.HasBar {
		return nil
	}
	pct := clampPercent(b.progress.Percent)
	width := 20
	if avail := m.contentWidth() - 22; avail < width {
		width = max(4, avail)
	}
	filled := int(pct / 100 * float64(width))
	bar := m.theme.BarFill.Render(strings.Repeat("█", filled)) +
		m.theme.BarEmpty.Render(strings.Repeat("░", max(0, width-filled)))
	return []string{"  " + bar + " " + m.theme.Detail.Render(fmt.Sprintf("%3.0f%%", pct))}
}

// renderSummary is the compact one-line outcome of a finished execution.
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

	elapsed := duration(b.elapsed())
	// A failed run should say why without the user having to expand it.
	reason := ""
	if b.err != "" {
		reason = " · " + truncate(b.err, max(12, m.contentWidth()-34))
	}
	// Findings are the outcome a security tool exists to produce, so they are
	// named on the summary line rather than only inside the expanded group.
	note := ""
	if n := len(b.findings); n > 0 {
		note = fmt.Sprintf(" · %s", plural(n, "finding", "findings"))
		if b.status == StatusFailed {
			note = ""
		}
	}
	return "  " + mark + " " + style.Render(verb+" in "+elapsed) + reason +
		m.theme.Detail.Render(note)
}

// renderGroups renders the findings and artifacts of a finished execution as
// compact trees, the way a terminal agent lists work it did.
func (m model) renderGroups(b *block) []string {
	var lines []string

	if len(b.findings) > 0 {
		fs := make([]Finding, len(b.findings))
		copy(fs, b.findings)
		sortFindings(fs)
		lines = append(lines, "  "+m.theme.Group.Render("Findings"))
		for i, f := range fs {
			lines = append(lines, m.renderFinding(f, i == len(fs)-1))
		}
	}

	if len(b.artifacts) > 0 {
		lines = append(lines, "  "+m.theme.Group.Render("Artifacts"))
		for i, a := range b.artifacts {
			lines = append(lines, m.renderArtifact(a, i == len(b.artifacts)-1))
		}
	}
	return lines
}

func (m model) renderFinding(f Finding, last bool) string {
	branch := treeBranch
	if last {
		branch = treeLeaf
	}
	tag := m.theme.severityStyle(f.Severity).Render(m.theme.severityTag(f.Severity))

	// The title is padded so every target starts in the same column. A ragged
	// target position is what makes a list of findings hard to scan, and
	// scanning them quickly is the whole point of showing them.
	titleWidth := 34
	if avail := m.contentWidth() - 14; avail < titleWidth {
		titleWidth = max(8, avail)
	}
	line := "  " + m.theme.Detail.Render(branch) + " " + tag + "  " +
		m.theme.Value.Render(pad(truncate(f.Title, titleWidth), titleWidth))
	if f.Target != "" {
		line += " " + m.theme.Detail.Render(truncate(f.Target, max(8, m.contentWidth()-titleWidth-12)))
	}
	return clampLine(line, m.contentWidth())
}

func (m model) renderArtifact(a Artifact, last bool) string {
	branch := treeBranch
	if last {
		branch = treeLeaf
	}
	line := "  " + m.theme.Detail.Render(branch) + " " + m.theme.Value.Render(a.Name)
	if a.Size != "" {
		line += " " + m.theme.Detail.Render(a.Size)
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
	head := "  " + m.theme.Group.Render("Events") + " " + m.theme.Detail.Render(plural(len(b.rows), "row", "rows"))
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
		return []string{"  " + m.theme.Group.Render("Output") + " " +
			m.theme.Hint.Render(note+" · expand with F2/enter")}
	}
	// A printed block is quoted under a rule rather than mixed into the tree:
	// the tree is for results the interface understood, and this is the tool
	// speaking for itself.
	lines := []string{"  " + m.theme.Group.Render("Output")}
	if b.outputOmitted {
		lines = append(lines, "  "+m.theme.Detail.Render("(truncated to the last "+plural(outputCap, "line", "lines")+")"))
	}
	width := m.contentWidth() - 2
	for _, raw := range b.output {
		for _, line := range wrapText(raw, width) {
			lines = append(lines, "  "+m.theme.Output.Render(line))
		}
	}
	return lines
}

// wrapText breaks a line at width, preferring spaces so words stay intact.
// A single word longer than the width is hard-split rather than dropped,
// because losing content silently is worse than an awkward break.
func wrapText(s string, width int) []string {
	s = strings.ReplaceAll(strings.TrimRight(s, "\r"), "\t", "    ")
	if width < 8 {
		return []string{s}
	}
	if len([]rune(s)) <= width {
		return []string{s}
	}
	var out []string
	var line []rune
	for _, word := range strings.Fields(s) {
		switch {
		case len(line) == 0:
			line = []rune(word)
		case len(line)+1+len([]rune(word)) <= width:
			line = append(line, ' ')
			line = append(line, []rune(word)...)
		default:
			out = append(out, string(line))
			line = []rune(word)
		}
		// A single token wider than the line: break it by hand.
		for len(line) > width {
			out = append(out, string(line[:width]))
			line = line[width:]
		}
	}
	if len(line) > 0 {
		out = append(out, string(line))
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
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

// viewportHeight is how many rows the transcript owns between the header rule
// and the composer.
//
// The fixed furniture is four rows: the header, its rule, the composer's rule,
// and the composer. Once there is any execution in the session another row goes
// to the activity strip between the transcript and the composer -- the line
// that reports new lines below the fold while the operator is reading history.
// The strip is reserved rather than drawn on demand so the viewport does not
// resize every time follow mode toggles: a viewport that changes height on a
// scroll would move the transcript under the fingers that are using it.
func (m *model) viewportHeight(termHeight int) int {
	rows := termHeight - 4
	if len(m.blocks) > 0 {
		rows--
	}
	return max(1, rows)
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
	entries := m.caps.Entries()
	// Fit as many complete capabilities as the terminal height allows, counting
	// the two lines each takes. A row cut in half by the region's edge is worse
	// than one capability less.
	budget := max(2, m.height-10)
	shown := 0
	for _, e := range entries {
		if shown+2 > budget {
			break
		}
		shown++
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
		r.Add(style.Render(clampLine(marker+" "+e.Name, width-2)))
		if e.Detail != "" {
			r.Add(m.theme.Detail.Render(clampLine("  "+e.Detail, width-2)))
		}
		if e.Note != "" && e.Note != "live provider" {
			r.Add(m.theme.Hint.Render(clampLine("  "+e.Note, width-2)))
		}
	}
	if len(entries) > shown {
		remaining := len(entries) - shown
		r.Add(m.theme.Hint.Render(clampLine(fmt.Sprintf("  +%d more", remaining), width-2)))
	}
	return r
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
