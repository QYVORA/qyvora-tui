package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// execEntry is one row of the executions region: a command the operator ran,
// its outcome, and -- when expanded -- the detail beneath its header.
//
// Entries are ordered newest first, so the running command (always the newest)
// sits at the top of the region and the session's history reads in reverse.
type execEntry struct {
	block  *block
	header string
	detail []string
}

// execEntries builds the executions region's rows from the session's blocks,
// newest first, rendered at the given width.
func (m model) execEntries(width int) []execEntry {
	entries := make([]execEntry, 0, len(m.blocks))
	for i := len(m.blocks) - 1; i >= 0; i-- {
		b := m.blocks[i]
		entry := execEntry{
			block:  b,
			header: m.execEntryHeader(b, width, i),
			detail: m.execEntryDetail(b, width),
		}
		entries = append(entries, entry)
	}
	return entries
}

// execEntryHeader is one execution's line in the region: an expand glyph, the
// outcome, the command, and its duration.
func (m model) execEntryHeader(b *block, width, index int) string {
	glyph := "▸"
	if b.expanded {
		glyph = "▾"
	}
	symStyle := m.theme.Detail
	sym := statusSymbol(b)
	switch b.status {
	case StatusRunning:
		symStyle = m.theme.Running
	case StatusDone:
		symStyle = m.theme.Success
	case StatusFailed:
		symStyle = m.theme.Failed
	case StatusCancelled:
		symStyle = m.theme.Cancelled
	}

	selected := m.regionFocus && index == m.execSel
	style := m.theme.Detail
	cmdStyle := m.theme.Value
	if selected {
		// The focused row is drawn in the selection style so the keyboard has a
		// visible position even before the operator presses enter.
		style = m.theme.Selection
		cmdStyle = m.theme.Selection
	}

	prefix := style.Render(glyph+" ") + symStyle.Render(sym) + " "
	tail := " " + style.Render(duration(b.elapsed()))
	avail := width - lipgloss.Width(prefix) - lipgloss.Width(tail)
	if avail < 4 {
		avail = 4
	}
	line := prefix + cmdStyle.Render(truncate(b.command, avail)) + tail
	return clampLine(line, width)
}

// statusSymbol is the outcome glyph an execution entry leads with.
func statusSymbol(b *block) string {
	switch b.status {
	case StatusRunning:
		return "⠿"
	case StatusDone:
		return "✓"
	case StatusFailed:
		return "✕"
	case StatusCancelled:
		return "■"
	}
	return "·"
}

// execEntryDetail is the body beneath an entry's header. A running command
// carries its live state -- stage, counts, errors -- because that is the
// region's whole purpose while work is in flight. A finished command shows
// nothing until it is expanded, which is what keeps a long history scannable;
// expanding it reveals its tally and, for a failure, the reason.
func (m model) execEntryDetail(b *block, width int) []string {
	if b.status == StatusRunning {
		return m.runningDetail(b, width)
	}
	if !b.expanded {
		return nil
	}
	var lines []string
	if b.err != "" {
		lines = append(lines, m.theme.Failed.Render(clampLine("  ✗ "+truncate(b.err, max(8, width-4)), width)))
		return lines
	}
	if n := len(b.findings); n > 0 {
		lines = append(lines, m.theme.Success.Render(clampLine("  ✓ "+plural(n, "finding", "findings")+" · "+plural(len(b.artifacts), "artifact", "artifacts"), width)))
	} else if n := len(b.artifacts); n > 0 {
		lines = append(lines, m.theme.Value.Render(clampLine("  "+plural(n, "artifact", "artifacts"), width)))
	}
	if n := b.known; n > 0 {
		lines = append(lines, m.theme.Detail.Render(clampLine("  "+plural(n, "event", "events"), width)))
	}
	if len(lines) == 0 {
		// "No output" was the wrong sentence here. A run that printed a
		// capability table or a scan listing has output on screen -- it simply
		// recorded no findings, artifacts or events to summarise. Saying
		// "no output" about a run whose output was expanded two inches to the
		// left is the kind of line that makes an operator stop trusting the
		// panel, so the two cases are named separately.
		if n := len(b.output); n > 0 {
			lines = append(lines, m.theme.Detail.Render(
				clampLine("  "+plural(n, "line", "lines")+" printed", width)))
		} else {
			lines = append(lines, m.theme.Detail.Render(clampLine("  no output", width)))
		}
	}
	return lines
}

// runningDetail is the live state of the execution currently in flight. It is
// ported from the old activity region, which the navigator absorbs: the counts
// and the recent tail are exactly the facts an operator wants while a scan
// runs, and a finished region that kept showing them would be one that never
// moved on.
func (m model) runningDetail(b *block, width int) []string {
	a := m.activity
	if a == nil || a.Total == 0 {
		return []string{m.theme.Detail.Render(clampLine("  starting…", width))}
	}
	var lines []string
	if stage := a.stageLine(); stage != "" {
		lines = append(lines, m.theme.Value.Render(clampLine("  "+stage, width)))
	}
	if errs := a.errorCount(); errs > 0 {
		lines = append(lines, m.theme.Failed.Render(clampLine("  ✗ "+plural(errs, "error", "errors"), width)))
	}
	events := a.Total
	lines = append(lines, m.theme.Detail.Render(clampLine(fmt.Sprintf("  %d events", events), width)))
	if a.Findings > 0 {
		lines = append(lines, m.theme.Success.Render(clampLine("  ✓ "+plural(a.Findings, "finding", "findings"), width)))
	}
	if n := len(a.Recent); n > 0 {
		last := a.Recent[n-1]
		if b.status == StatusRunning && len(a.Recent) > 1 {
			last = a.Recent[n-2]
		}
		lines = append(lines, m.theme.Activity.Render(clampLine("  "+humanEvent(last), width)))
	}
	return lines
}

// errorCount sums every event level that counts as a failure, the number the
// region leads with so a problem is never buried under a running tally.
func (a *Activity) errorCount() int {
	if a == nil {
		return 0
	}
	total := 0
	for level, n := range a.ByLevel {
		if isErrorLevel(level) {
			total += n
		}
	}
	return total
}

// entryHeight is how many region rows one entry occupies: its header plus any
// detail.
func entryHeight(e execEntry) int { return 1 + len(e.detail) }

// expandedRegionWindow slices the region's entries to the column's height,
// honouring the region's own scroll offset, so a long session scrolls inside
// its panel rather than growing past the composer.
func (m model) expandedRegionWindow(entries []execEntry) (rows []string, more bool) {
	capacity := m.viewport.Height
	var all []string
	for _, e := range entries {
		all = append(all, e.header)
		all = append(all, e.detail...)
	}
	off := clamp(m.execOffset, 0, max(0, len(all)-1))
	end := off + capacity
	if end >= len(all) {
		end = len(all)
		off = max(0, end-capacity)
	}
	if end < len(all) {
		more = true
	}
	return all[off:end], more
}

// canFocusRegion reports whether the executions region exists and can take
// the keyboard. It resolves the layout rather than checking a stored flag, so
// a terminal resized while the region had focus decides freshly.
func (m model) canFocusRegion() bool {
	if len(m.blocks) == 0 || m.width < CompactWidth || m.height < minRegionHeight {
		return false
	}
	l := m.resolveLayout(m.width)
	return l.Activity > 0 && m.executionsRegion(l.Activity) != nil
}

// handleRegionKey routes a keystroke to the executions region.
//
// Arrow keys move the selection between executions, Enter and Space expand and
// collapse the selected execution in both the region and the transcript, and
// every other key returns to the composer untouched.
func (m model) handleRegionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	width := m.layout.Activity
	entries := m.execEntries(width)
	total := len(entries)
	switch msg.Type {
	case tea.KeyUp:
		if m.execSel > 0 {
			m.execSel--
		}
	case tea.KeyDown:
		if m.execSel < total-1 {
			m.execSel++
		}
	case tea.KeyHome, tea.KeyCtrlA:
		m.execSel = 0
	case tea.KeyEnd, tea.KeyCtrlE:
		m.execSel = max(0, total-1)
	case tea.KeyPgUp:
		m.execOffset -= m.viewport.Height / 2
	case tea.KeyPgDown:
		m.execOffset += m.viewport.Height / 2
	case tea.KeyEnter, tea.KeySpace:
		if total > 0 && m.execSel >= 0 && m.execSel < total {
			m.toggleExec(entries[m.execSel].block)
		}
		return m, nil
	}
	if total > 0 {
		m.ensureEntryVisible(entries, m.execSel)
	}
	m.refreshViewport()
	return m, nil
}

// toggleExec expands or collapses an execution across both the transcript and
// the region, and scrolls the transcript so the execution becomes visible. The
// pair is what makes the region a navigator rather than a separate list: an
// expansion is one gesture that says "show me that run".
func (m *model) toggleExec(b *block) {
	if b == nil || b.status == StatusRunning {
		return
	}
	b.expanded = !b.expanded
	m.refreshViewport()
	m.revealBlock(b.id)
}

// revealBlock scrolls the transcript to the start of an execution, so acting
// on a row in the navigator lands the operator at that run.
func (m *model) revealBlock(id int) {
	idx := -1
	for i, b := range m.blocks {
		if b.id == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	line := 0
	for i := 0; i < idx; i++ {
		line += len(m.renderBlock(m.blocks[i])) + 1
	}
	m.viewport.YOffset = clamp(line, 0, max(0, m.viewport.TotalLineCount()-m.viewport.Height))
	m.syncFollow()
}

// ensureEntryVisible scrolls the region so the selected entry's header is on
// screen.
func (m *model) ensureEntryVisible(entries []execEntry, sel int) {
	if sel < 0 || sel >= len(entries) {
		return
	}
	start := 0
	for i := 0; i < sel; i++ {
		start += entryHeight(entries[i])
	}
	cap := m.viewport.Height
	if start < m.execOffset {
		m.execOffset = start
		return
	}
	if end := start + entryHeight(entries[sel]); end > m.execOffset+cap {
		m.execOffset = end - cap
	}
	if m.execOffset < 0 {
		m.execOffset = 0
	}
}

// execScroll scrolls the region by a delta of rows, clamped to its content.
func (m *model) execScroll(delta int) {
	width := m.layout.Activity
	all := m.entryRows(m.execEntries(width))
	m.execOffset = clamp(m.execOffset+delta, 0, max(0, len(all)-1))
}

// entryRows flattens a set of entries back into the region's line stream.
func (m model) entryRows(entries []execEntry) []string {
	var all []string
	for _, e := range entries {
		all = append(all, e.header)
		all = append(all, e.detail...)
	}
	return all
}

// handleMouse routes a mouse event. The wheel scrolls the transcript or the
// executions region, depending on the column; a click selects and expands an
// execution, jumps to the newest output on the activity strip, or returns the
// focus to the composer.
//
// The coordinates bubbletea reports are cells of the alternate screen, so the
// pointers here are row and column indexes of the very view the render path
// draws -- which is why the geometry is computed from the same layout the view
// used.
func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.form != nil {
		return m, nil
	}
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	y, x := msg.Y, msg.X

	switch {
	case msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown:
		// Scrolling down reads further into the history, which for the region
		// means the first visible entry moves down the list; for the transcript
		// it means the content moves up through the fold.
		delta := 3
		if msg.Button == tea.MouseButtonWheelUp {
			delta = -delta
		}
		if m.overBody(y) && m.overRegionColumn(x) {
			m.execScroll(delta)
			return m, nil
		}
		if m.overBody(y) {
			if delta < 0 {
				m.scrollUp(-delta)
			} else {
				m.scrollDown(delta)
			}
		}
		return m, nil

	case msg.Button == tea.MouseButtonLeft && y == m.composerInputRow():
		// Clicking the input bar returns the keyboard to the composer from
		// wherever region focus took it.
		m.regionFocus = false
		m.refreshViewport()
		return m, nil

	case msg.Button == tea.MouseButtonLeft && m.overStrip(y) && !m.following:
		// The activity strip is the "new output is waiting" notice; clicking
		// it does what ctrl+end does.
		m.following = true
		m.gotoBottom()
		m.refreshViewport()
		return m, nil

	case msg.Button == tea.MouseButtonLeft && m.overBody(y) && m.overRegionColumn(x):
		m.regionRowClick(x, y-m.bodyStart())
		m.refreshViewport()
		return m, nil

	case msg.Button == tea.MouseButtonLeft:
		m.regionFocus = false
		m.refreshViewport()
		return m, nil
	}
	return m, nil
}

// regionRowClick resolves a click inside the executions region onto an entry:
// a click on an entry's header row selects it and toggles its expansion, a
// click on its detail selects it. Rows past the end of the list are ignored.
func (m *model) regionRowClick(x, row int) {
	width := m.layout.Activity
	entries := m.execEntries(width)
	if len(entries) == 0 {
		return
	}
	off := m.execOffset
	all := m.entryRows(entries)
	if off > len(all) {
		off = 0
	}
	// The first two rows of the region are its title and its rule; the entries
	// begin at row 2, so a click on the entry stream is two rows in before the
	// list itself starts.
	target := row - 2 + off
	at := 0
	entry, sub := -1, -1
	for i, e := range entries {
		h := entryHeight(e)
		if target >= at && target < at+h {
			entry, sub = i, target-at
			break
		}
		at += h
	}
	if entry < 0 {
		return
	}
	m.execSel = entry
	m.execOffset = off
	if sub == 0 {
		m.toggleExec(entries[entry].block)
	} else {
		m.ensureEntryVisible(entries, entry)
	}
}

// The row-and-column geometry below is the same arithmetic the render path
// uses, kept next to the mouse handler so a layout change cannot drift the two
// apart.

// bodyStart is the first body row: the header rule is row 1, and the body the
// viewport and regions occupy begins at row 2.
func (m model) bodyStart() int { return 2 }

// bodyEnd is one past the last body row.
func (m model) bodyEnd() int { return m.bodyStart() + m.viewport.Height }

// overBody reports whether a row is inside the transcript/region body.
func (m model) overBody(y int) bool { return y >= m.bodyStart() && y < m.bodyEnd() }

// stripShown reports whether the activity strip row exists. It is reserved for
// the whole rest of the session once anything has run, so follow mode never
// resizes the viewport.
func (m model) stripShown() bool { return len(m.blocks) > 0 && m.form == nil }

// stripRow is the row the strip occupies.
func (m model) stripRow() int { return m.bodyEnd() }

// overStrip reports whether a row is the activity strip.
func (m model) overStrip(y int) bool { return m.stripShown() && y == m.stripRow() }

// composerInputRow is the row the composer's input line occupies.
func (m model) composerInputRow() int {
	r := m.bodyEnd()
	if m.stripShown() {
		r++
	}
	return r + 1
}

// regionXRange is the executions region's column span, when drawn.
func (m model) regionXRange() (start, end int, present bool) {
	if m.layout.Activity <= 0 || m.executionsRegion(m.layout.Activity) == nil {
		return 0, 0, false
	}
	return m.width - (m.layout.Activity + regionChrome), m.width, true
}

// overRegionColumn reports whether a column falls inside the executions
// region, the only interactive region.
func (m model) overRegionColumn(x int) bool {
	start, _, present := m.regionXRange()
	return present && x >= start
}
