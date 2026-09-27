package tui

import (
	"fmt"
	"strings"
	"time"

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
	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	b.WriteString(m.renderComposer())
	return b.String()
}

// renderHeader draws the tool identity and session state on one line.
func (m model) renderHeader() string {
	title := m.cfg.Title
	if title == "" {
		title = "QYVORA"
	}
	if m.cfg.Version != "" {
		title += "  " + m.cfg.Version
	}

	left := m.theme.Title.Render(title)

	right := ""
	switch {
	case m.running:
		right = m.theme.Running.Render(spinnerFrames[m.spinner] + " running " + duration(time.Since(m.started)))
	case len(m.notices) > 0 && m.lastNoticeIsError():
		right = m.theme.Failed.Render("error")
	case m.following == false:
		// The user has scrolled back; say so, because new output is arriving
		// below the fold and silently not being seen is disorienting.
		right = m.theme.Hint.Render("paused · Ctrl+End to follow")
	default:
		right = m.theme.Ready.Render("ready")
	}

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		// Too narrow to sit side by side. The identity matters more than the
		// status, so the status gives way rather than both being clipped.
		return clampLine(left, m.width)
	}
	return clampLine(left+strings.Repeat(" ", gap)+right, m.width)
}

func (m model) lastNoticeIsError() bool {
	if len(m.notices) == 0 {
		return false
	}
	return strings.HasPrefix(m.notices[len(m.notices)-1], "Error:") ||
		strings.Contains(m.notices[len(m.notices)-1], "failed")
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
	b.WriteString(clampLine(line, m.width))
	return b.String()
}

// hint returns the context-sensitive key hint on the right of the composer.
func (m model) hint() string {
	switch {
	case m.running:
		return "ctrl+c stop"
	case m.following:
		return "tab complete · ↑↓ history · ctrl+o events"
	default:
		return "ctrl+end latest"
	}
}

// transcriptLines renders the whole session as terminal lines.
//
// The transcript is a flat list of lines rather than a set of widgets, which is
// what lets the viewport scroll it and the composer stay put. A completed
// command collapses to a compact summary; a running one shows its live state.
func (m model) transcriptLines() []string {
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
	out := m.theme.Prompt.Render("> ") + m.renderCommandText(b.command, m.contentWidth())
	return []string{out, "  "}
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

	// A running command leads with what it is doing right now, which is the
	// only thing that changes while it works.
	if b.status == StatusRunning {
		lines = append(lines, m.renderLive(b))
		lines = append(lines, m.renderProgress(b)...)
		return lines
	}

	lines = append(lines, m.renderSummary(b))
	lines = append(lines, m.renderGroups(b)...)
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
	lines := []string{"  " + m.theme.Group.Render("Events")}
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
func (m model) contentWidth() int {
	return max(24, m.width-2)
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
