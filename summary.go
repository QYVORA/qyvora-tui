package tui

import (
	"sort"
	"strconv"
	"strings"

	"github.com/QYVORA/qyvora-tui/internal/layout"

	"github.com/charmbracelet/lipgloss"
)

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
	target = layout.ClampInt(avail/3, minFindingTarget, maxFindingTarget)
	title = max(minFindingTitle, avail-target-1)
	return title, target
}

// artifactSizeWidth is the column artifact sizes share, so a list of files
// lines up by size the way findings line up by target.
func (m model) artifactSizeWidth() int {
	const fixed = 2 + 1 + 1 + 1
	return layout.ClampInt(m.contentWidth()-fixed, 0, 12)
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
