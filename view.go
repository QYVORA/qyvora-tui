package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// View renders the whole interface: header, transcript, and the input line.
//
// The layout is computed from the current terminal size on every render.
// Nothing about the width is baked in, so a resize is handled by the next
// frame rather than needing a special path.
func (m model) View() string {
	if m.quitting {
		return ""
	}
	if m.width <= 0 || m.height <= 0 {
		// Before the first WindowSizeMsg there is no size to lay out
		// against; render the input alone rather than guessing.
		return m.renderInput()
	}

	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n")
	b.WriteString(m.renderDivider())
	b.WriteString("\n")

	// The transcript fills whatever is left after the header, divider and
	// input line, so the scroll viewport never overlaps the prompt.
	viewHeight := m.height - 3
	if viewHeight < 1 {
		viewHeight = 1
	}
	transcript := m.renderTranscript(viewHeight)
	b.WriteString(transcript)
	b.WriteString("\n")
	b.WriteString(m.renderDivider())
	b.WriteString("\n")
	b.WriteString(m.renderInput())
	return b.String()
}

// renderHeader draws the title and the current status.
func (m model) renderHeader() string {
	title := m.cfg.Title
	if title == "" {
		title = "QYVORA / " + strings.ToUpper(m.runner.Name())
	}
	if m.cfg.Version != "" {
		title += "  " + m.cfg.Version
	}

	label := m.state.String()
	if m.state == stateRunning {
		label = spinnerFrames[m.spinner] + " " + label
	}
	if m.running && !m.started.IsZero() {
		label += "  " + duration(time.Since(m.started))
	}

	dot := m.theme.statusStyle(m.statusForDot()).Render("●")

	left := m.theme.HeaderTitle.Render(title)
	right := m.theme.statusStyle(m.statusForDot()).Render(label)

	// Both the single-row and stacked layouts are clamped: at a very narrow
	// width a long tool name would otherwise push the status off screen or
	// wrap, breaking the header's alignment.
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		// Too narrow for a comfortable row: stack the status under the title
		// rather than truncating either into unreadability.
		return clampLine(left, m.width) + "\n" + clampLine(dot+" "+right, m.width)
	}
	return clampLine(left+strings.Repeat(" ", gap)+dot+" "+right, m.width)
}

func (m model) statusForDot() Status {
	switch m.state {
	case stateRunning:
		return StatusRunning
	case stateFailed:
		return StatusFailed
	case stateCancelled:
		return StatusCancelled
	default:
		return StatusDone
	}
}

func (m model) renderDivider() string {
	return m.theme.Border.Render(strings.Repeat("─", max(1, m.width)))
}

// renderInput draws the prompt line and the key hints.
func (m model) renderInput() string {
	input := m.input.View()
	line := input

	if hint := m.hint(); hint != "" {
		hint = m.theme.Hint.Render(hint)
		space := m.width - lipgloss.Width(line) - lipgloss.Width(hint)
		if space >= 2 {
			line += strings.Repeat(" ", space) + hint
		}
	}
	return clampLine(line, m.width)
}

// hint returns the context-sensitive key hint shown at the right of the prompt.
func (m model) hint() string {
	switch {
	case m.running:
		return "Ctrl+C stop"
	case m.blocks == nil:
		return "help  Ctrl+D quit"
	default:
		return "Tab complete  ↑↓ history"
	}
}

// renderTranscript lays out the session blocks into the available height,
// keeping the newest content visible.
func (m model) renderTranscript(height int) string {
	lines := m.transcriptLines()
	if len(lines) == 0 {
		lines = []string{m.theme.Dim.Render("Type a command, or help for what is available.")}
	}
	// Clip horizontally as well as vertically. A line wider than the terminal
	// would wrap and break the block layout, so every line is constrained to
	// the real width rather than trusted to fit.
	for i, l := range lines {
		lines[i] = clampLine(l, m.width)
	}
	if len(lines) > height {
		lines = lines[len(lines)-height:]
	}
	return strings.Join(lines, "\n")
}

// clampLine shortens a rendered line to the terminal width, preserving the
// styling already applied to its prefix.
func clampLine(line string, width int) string {
	if width <= 0 || lipgloss.Width(line) <= width {
		return line
	}
	// Truncating styled text by measuring visible width is fiddly; a plain
	// truncation is acceptable here because the clipped region is the tail of
	// a detail value, not the label the reader scans.
	r := []rune(stripANSI(line))
	if width >= 1 && len(r) > width {
		return string(r[:width-1]) + "…"
	}
	return line
}

// transcriptLines renders every block into a flat list of terminal lines.
//
// Rendering to lines first, then clipping, is what makes scrolling and
// resizing behave: the layout is computed for the real content, and the
// viewport simply shows the tail of it.
func (m model) transcriptLines() []string {
	var lines []string
	for i, blk := range m.blocks {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, m.renderBlock(blk)...)
	}
	if len(m.notices) > 0 {
		lines = append(lines, "")
		for _, n := range m.notices {
			lines = append(lines, n)
		}
	}
	return lines
}

// renderBlock renders one execution as a coherent block.
func (m model) renderBlock(b *block) []string {
	marker := "▸"
	heading := fmt.Sprintf("%s Execution", marker)
	switch b.status {
	case StatusDone:
		heading = marker + " Execution"
	case StatusFailed:
		heading = marker + " Execution  " + m.theme.Failed.Render("✗")
	case StatusCancelled:
		heading = marker + " Execution  " + m.theme.Cancelled.Render("⊘")
	}

	title := m.theme.BlockTitle.Render(heading)
	timing := m.theme.Dim.Render(duration(b.elapsed()))
	gap := m.width - lipgloss.Width(title) - lipgloss.Width(timing) - 2
	if gap < 1 {
		lines := []string{title, m.theme.Dim.Render("  " + duration(b.elapsed()))}
		lines = append(lines, m.renderBlockBody(b)...)
		return lines
	}

	lines := []string{title + strings.Repeat(" ", gap) + timing}
	lines = append(lines, m.renderBlockBody(b)...)
	return lines
}

// renderBlockBody renders the fields of a block beneath its heading.
func (m model) renderBlockBody(b *block) []string {
	var lines []string
	indent := "  "

	add := func(label, value string) {
		if value == "" {
			return
		}
		lines = append(lines, indent+m.theme.Label.Render(pad(label, 11))+m.theme.Value.Render(value))
	}

	add("Command", m.theme.Command.Render("$ "+b.command))
	add("Status", m.statusText(b))
	if b.status == StatusRunning && b.progress.Message != "" {
		add("Progress", m.renderProgress(b.progress))
	} else if b.progress.HasBar {
		add("Progress", m.renderProgress(b.progress))
	}
	if len(b.findings) > 0 {
		add("Findings", plural(len(b.findings), "finding", "findings"))
	}
	if len(b.artifacts) > 0 {
		add("Artifacts", plural(len(b.artifacts), "artifact", "artifacts"))
	}
	add("Events", plural(b.known, "event", "events"))
	if b.err != "" {
		lines = append(lines, indent+m.theme.Error.Render("Error     "+b.err))
	}

	lines = append(lines, m.renderBlockDetails(b)...)
	return lines
}

func (m model) statusText(b *block) string {
	switch b.status {
	case StatusRunning:
		return spinnerFrames[m.spinner] + " Running"
	case StatusDone:
		return m.theme.Success.Render("Done")
	case StatusFailed:
		if b.exitCode != 0 {
			return m.theme.Failed.Render(fmt.Sprintf("Failed (exit %d)", b.exitCode))
		}
		return m.theme.Failed.Render("Failed")
	case StatusCancelled:
		return m.theme.Cancelled.Render(fmt.Sprintf("Cancelled (exit %d)", b.exitCode))
	}
	return ""
}

// renderProgress draws a progress bar when the tool reported a percentage.
func (m model) renderProgress(p progressLine) string {
	if !p.HasBar {
		return p.Message
	}
	pct := clampPercent(p.Percent)
	barWidth := 24
	if m.width > 0 && m.width < 60 {
		// A narrow terminal cannot afford a long bar; shrink it rather than
		// letting the line wrap and break the block layout.
		barWidth = max(6, m.width/3)
	}
	filled := int(pct / 100 * float64(barWidth))
	bar := m.theme.BarFill.Render(strings.Repeat("█", filled)) +
		m.theme.BarEmpty.Render(strings.Repeat("░", barWidth-filled))
	out := fmt.Sprintf("%s %3.0f%%", bar, pct)
	if p.Message != "" {
		out += "  " + p.Message
	}
	return out
}

func clampPercent(p float64) float64 {
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

// renderBlockDetails renders the expanded sections of a block: the events, the
// findings and the artifacts.
//
// Raw event payloads are preserved here rather than only being summarised, so
// nothing the tool reported is lost to the display layer.
func (m model) renderBlockDetails(b *block) []string {
	var lines []string
	indent := "  "

	if len(b.findings) > 0 {
		lines = append(lines, indent+m.theme.Dim.Render("Findings"))
		fs := make([]Finding, len(b.findings))
		copy(fs, b.findings)
		sortFindings(fs)
		for _, f := range fs {
			lines = append(lines, m.renderFinding(f))
		}
	}

	if len(b.artifacts) > 0 {
		lines = append(lines, indent+m.theme.Dim.Render("Artifacts"))
		for _, a := range b.artifacts {
			line := a.Name
			if a.Kind != "" {
				line += m.theme.Dim.Render("  (" + a.Kind + ")")
			}
			if a.Size != "" {
				line += m.theme.Dim.Render("  " + a.Size)
			}
			lines = append(lines, indent+"  "+line)
		}
	}

	if m.expanded[b.id] {
		lines = append(lines, indent+m.theme.Dim.Render("Events"))
		for _, row := range b.rows {
			lines = append(lines, indent+"  "+m.renderRow(row))
			if row.Data != nil {
				for _, k := range sortedKeys(row.Data) {
					lines = append(lines, indent+"      "+
						m.theme.Label.Render(pad(k, 16))+
						m.theme.Dim.Render(truncate(dataString(row.Data, k), m.detailWidth())))
				}
			}
		}
	} else if b.known > 0 {
		lines = append(lines, indent+m.theme.Dim.Render(
			fmt.Sprintf("%s -- press Tab on the command line to inspect raw event data", b.summarise())))
	}
	return lines
}

func (m model) renderFinding(f Finding) string {
	line := "  "
	if f.Severity != "" {
		line += m.theme.severityStyle(f.Severity).Render(pad(f.Severity, 9))
	} else {
		line += strings.Repeat(" ", 9)
	}
	line += f.Title
	if f.Target != "" {
		line += m.theme.Dim.Render("  " + f.Target)
	}
	if f.Detail != "" {
		line += m.theme.Dim.Render("  " + truncate(f.Detail, max(20, m.detailWidth())))
	}
	return line
}

func (m model) renderRow(row eventRow) string {
	level := strings.ToUpper(row.Level)
	style := m.theme.Dim
	switch level {
	case "ERROR", "FATAL":
		style = m.theme.Error
	case "WARN", "WARNING":
		style = m.theme.Medium
	case "INFO":
		style = m.theme.Info
	}
	out := style.Render(pad(level, 7)) + m.theme.BlockTitle.Render(pad(row.Label, 26))
	if row.Detail != "" {
		out += m.theme.Dim.Render(truncate(row.Detail, max(20, m.detailWidth())))
	}
	return out
}

// detailWidth is how much room a detail value gets, derived from the terminal
// so long values wrap instead of overflowing.
func (m model) detailWidth() int {
	return max(24, m.width-34)
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
