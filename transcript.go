package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

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
