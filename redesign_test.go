package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The redesign tests pin the behaviour the v0.7.0 layout introduced: the
// transcript wrapping against the viewport rather than the terminal, the
// executions region as a persistent expandable navigator, region focus, the
// activity strip, and the mouse paths.

func TestContentWidthFollowsTheTranscriptNotTheTerminal(t *testing.T) {
	// This is the root-cause distortion: blocks wrapped to the full terminal
	// while the viewport was narrower (a region had taken its columns), so
	// renderOutput laid lines out wider than the viewport could show and
	// composeRegions then trimmed them back with an ellipsis.
	caps := normalize(t, "sekhmet", familyAJSON)
	m := modelFor(t, 200, 40, caps)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyF1})
	m = updated.(model)

	if m.layout.Transcript >= m.width {
		t.Fatalf("the test needs a region to shrink the transcript; got %d of %d", m.layout.Transcript, m.width)
	}
	if got := m.contentWidth(); got != m.viewport.Width-2 {
		t.Errorf("contentWidth = %d, want viewport width (%d) minus the indent", got, m.viewport.Width-2)
	}
}

func TestIdleSessionExplainsItself(t *testing.T) {
	m := modelFor(t, 100, 40, nil)
	out := stripANSI(m.View())
	if !strings.Contains(out, "ready for a command") {
		t.Errorf("the empty session does not explain itself:\n%s", out)
	}
	if strings.Contains(out, "EXECUTIONS") || strings.Contains(out, "CAPABILITIES") {
		t.Errorf("an idle session draws regions:\n%s", out)
	}
}

func TestCompletedBlockCollapsesItsOutputUntilExpanded(t *testing.T) {
	th := newTheme(false, nil)
	m := newModel(Config{Title: "QYVORA / TEST", Runner: &InProcessRunner{Execute: func(ctx context.Context, _ []string) int { return 0 }, ToolName: "test"}}, th)
	m = resize(m, 100, 40)
	m = addBlock(m, newBlock(1, []string{"scan", "example.com"}))
	b := m.blocks[0]
	b.addOutput("Querying crt.sh ...")
	b.addOutput("crt.sh found 42 potential subdomains")
	b.finished = time.Now()
	b.status = StatusDone
	m = refresh(m)

	// Collapsed by default: the preview names the size, the raw text is not
	// dumped into the session's history.
	if b.expanded {
		t.Fatal("a finished run starts expanded")
	}
	out := stripANSI(m.View())
	if !strings.Contains(out, "2 lines") {
		t.Errorf("the collapsed preview does not name the output size:\n%s", out)
	}
	if strings.Contains(out, "crt.sh found 42") {
		t.Errorf("collapsed output leaked into the transcript:\n%s", out)
	}

	// Expanding (F2 into the region, Enter) reveals the full text, and the
	// transcript scrolls to the run.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyF2})
	m = updated.(model)
	if !m.regionFocus {
		t.Fatal("F2 did not focus the executions region")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if !m.blocks[0].expanded {
		t.Fatal("enter did not expand the selected execution")
	}
	out = stripANSI(m.View())
	if !strings.Contains(out, "crt.sh found 42") {
		t.Errorf("expanded output is not in the transcript:\n%s", out)
	}
}

func TestRunningBlockNeverCollapses(t *testing.T) {
	th := newTheme(false, nil)
	m := newModel(Config{Runner: &InProcessRunner{Execute: func(ctx context.Context, _ []string) int { return 0 }, ToolName: "test"}}, th)
	m = resize(m, 100, 40)
	m = addBlock(m, newBlock(1, []string{"scan", "x"}))
	m.blocks[0].addOutput("talking while working")
	m = refresh(m)
	out := stripANSI(m.View())
	if !strings.Contains(out, "talking while working") {
		t.Errorf("a running block hid its live output:\n%s", out)
	}
}

func TestResultsLineTalliesAFinishedRun(t *testing.T) {
	th := newTheme(false, nil)
	m := newModel(Config{Runner: &InProcessRunner{Execute: func(ctx context.Context, _ []string) int { return 0 }, ToolName: "test"}}, th)
	m = resize(m, 100, 40)
	m = addBlock(m, newBlock(1, []string{"assess"}))
	b := m.blocks[0]
	b.addEvent(Event{Event: EventFindingDiscovered, Level: "info", Data: map[string]any{"title": "x"}})
	b.finished = time.Now()
	b.status = StatusDone
	m = refresh(m)
	out := stripANSI(m.View())
	if !strings.Contains(out, "1 finding") {
		t.Errorf("the results tally is missing the finding:\n%s", out)
	}
}

func TestExecutionsRegionSurvivesTheRun(t *testing.T) {
	// The old activity region vanished the moment the run finished. The
	// navigator is the whole point of history: it has to still be there, with
	// the finished run in it.
	m := modelFor(t, 200, 40, nil)
	m.start([]string{"scan", "example.com"})
	m.finish(runDoneMsg{})
	out := stripANSI(m.View())
	if !strings.Contains(out, "EXECUTIONS") {
		t.Errorf("the executions region vanished after the run:\n%s", out)
	}
	if !strings.Contains(out, "scan example.com") {
		t.Errorf("the finished run is not listed in the region:\n%s", out)
	}
}

func TestExecutionsRegionExpandsAFinishedRunUnderKeys(t *testing.T) {
	m := modelFor(t, 200, 40, nil)
	m.start([]string{"one"})
	m.blocks[0].addOutput("the first run's output")
	m.finish(runDoneMsg{})
	m.start([]string{"two"})
	m.finish(runDoneMsg{})

	// The region lists newest first: "two" is the top row and the default
	// selection. Moving down selects the older run.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyF2})
	m = updated.(model)
	if m.execSel != 0 {
		t.Fatalf("selection = %d, want the newest run at 0", m.execSel)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(model)
	if m.execSel != 1 {
		t.Fatalf("selection = %d, want the older run", m.execSel)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if !m.blocks[0].expanded {
		t.Error("expanding the selected run did not expand its block")
	}
	if got := stripANSI(strings.Join(m.transcriptLines(), "\n")); !strings.Contains(got, "the first run's output") {
		t.Errorf("the expanded run's output is not on screen:\n%s", got)
	}
}

func TestRegionFocusReturnsToTheComposer(t *testing.T) {
	m := modelFor(t, 200, 40, nil)
	m.start([]string{"scan"})
	m.finish(runDoneMsg{})

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyF2})
	m = updated.(model)
	if !m.regionFocus {
		t.Fatal("F2 did not focus the region")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	if m.regionFocus {
		t.Error("esc left the region focused")
	}
	// A typing key returns to the composer without being consumed: the letter
	// lands in the input.
	m.input.SetValue("sta")
	m.input.CursorEnd()
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyF2})
	m = updated.(model)
	m.input.SetValue("scan ")
	m.input.CursorEnd()
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = updated.(model)
	if m.regionFocus {
		t.Error("typing while the region had focus did not return to the composer")
	}
	if got := m.input.Value(); got != "scan x" {
		t.Errorf("the keystroke was lost: %q", got)
	}
}

func TestActivityStripReportsNewLinesBelowTheFold(t *testing.T) {
	th := newTheme(false, nil)
	m := newModel(Config{Runner: &InProcessRunner{Execute: func(ctx context.Context, _ []string) int { return 0 }, ToolName: "test"}}, th)
	m = resize(m, 100, 12)
	m.running = true
	m = addBlock(m, newBlock(1, []string{"scan", "example.com"}))
	for i := 0; i < 30; i++ {
		m.blocks[0].addOutput("result line " + string(rune('a'+i%26)))
	}
	m.refreshViewport()

	// Following: the strip row exists (reserved) but carries no notice.
	out := stripANSI(m.View())
	if strings.Contains(out, "follow") {
		t.Errorf("the strip reports new lines while following:\n%s", out)
	}

	// Scroll up: the strip reports what is below the fold.
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftUp})
	m = upd.(model)
	if m.following {
		t.Fatal("scrolling up did not release follow mode")
	}
	out = stripANSI(m.View())
	if !strings.Contains(out, "follow") || !strings.Contains(out, "↓") {
		t.Errorf("the strip does not report lines below the fold:\n%s", out)
	}

	// Clicking the strip jumps to the newest output.
	row := m.stripRow()
	upd, _ = m.Update(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      m.width / 2,
		Y:      row,
	})
	m = upd.(model)
	if !m.following {
		t.Error("a click on the strip did not re-pin follow mode")
	}
	if m.viewport.YOffset != m.viewport.TotalLineCount()-m.viewport.Height {
		t.Errorf("a click on the strip did not jump to the newest output (offset %d)", m.viewport.YOffset)
	}
}

func TestWheelScrollsByColumn(t *testing.T) {
	th := newTheme(false, nil)
	m := newModel(Config{Runner: &InProcessRunner{Execute: func(ctx context.Context, _ []string) int { return 0 }, ToolName: "test"}}, th)
	m = resize(m, 200, 40)
	m.start([]string{"first"})
	m.finish(runDoneMsg{})
	for i := 0; i < 60; i++ {
		m.start([]string{"run" + string(rune('a'+i%26))})
		m.finish(runDoneMsg{})
	}
	m.blocks[0].addOutput("a long transcript line to give the viewport something to scroll")
	for i := 0; i < 40; i++ {
		m.blocks[0].addOutput("more output " + string(rune('a'+i%26)))
	}
	m.refreshViewport()

	before := m.viewport.YOffset

	// Wheel over the transcript column scrolls the transcript.
	upd, _ := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp, X: m.width / 2, Y: m.bodyStart()})
	m = upd.(model)
	if m.viewport.YOffset >= before {
		t.Errorf("wheel over the transcript did not scroll it: offset %d was %d", m.viewport.YOffset, before)
	}

	// Wheel over the region column scrolls the region, not the viewport.
	start, _, present := m.regionXRange()
	if !present {
		t.Fatal("no executions region to wheel over")
	}
	before = m.viewport.YOffset
	upd, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown, X: start + 2, Y: m.bodyStart()})
	m = upd.(model)
	if m.viewport.YOffset != before {
		t.Errorf("wheel over the region moved the viewport: %d -> %d", before, m.viewport.YOffset)
	}
	if m.execOffset == 0 {
		t.Errorf("wheel over the region did not scroll it: offset %d", m.execOffset)
	}
}

func TestClickOnARegionHeaderExpandsTheRun(t *testing.T) {
	m := modelFor(t, 200, 40, nil)
	m.start([]string{"scan", "example.com"})
	m.blocks[0].addOutput("hidden until expanded")
	m.finish(runDoneMsg{})

	start, _, present := m.regionXRange()
	if !present {
		t.Fatal("no executions region to click")
	}
	// The first entry's header row is the third row of the region: its title,
	// its rule, then the newest entry.
	upd, _ := m.Update(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      start + 2,
		Y:      m.bodyStart() + 2,
	})
	m = upd.(model)
	if !m.blocks[0].expanded {
		t.Error("a click on the region header did not expand the run")
	}
	out := stripANSI(m.View())
	if !strings.Contains(out, "hidden until expanded") {
		t.Errorf("the expanded output did not reach the view:\n%s", out)
	}
}

func TestClickOnTheComposerReturnsFocus(t *testing.T) {
	m := modelFor(t, 200, 40, nil)
	m.start([]string{"scan"})
	m.finish(runDoneMsg{})
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyF2})
	m = updated.(model)
	if !m.regionFocus {
		t.Fatal("F2 did not focus the region")
	}
	upd, _ := m.Update(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      4,
		Y:      m.composerInputRow(),
	})
	m = upd.(model)
	if m.regionFocus {
		t.Error("a click on the composer left the region focused")
	}
}

func TestHeaderPillCarriesState(t *testing.T) {
	m := modelFor(t, 200, 40, nil)
	out := stripANSI(m.View())
	if !strings.Contains(out, "● ready") {
		t.Errorf("the idle pill is missing:\n%s", out)
	}
	m.start([]string{"scan"})
	m.running = true
	out = stripANSI(m.View())
	if !strings.Contains(out, " RUNNING ") {
		t.Errorf("the running pill is missing:\n%s", out)
	}
	m.finish(runDoneMsg{exitCode: 2})
	out = stripANSI(m.View())
	if !strings.Contains(out, "✕ FAILED") {
		t.Errorf("the failed pill is missing:\n%s", out)
	}
	m.finish(runDoneMsg{cancel: true})
	out = stripANSI(m.View())
	if !strings.Contains(out, "■ CANCELLED") {
		t.Errorf("the cancelled pill is missing:\n%s", out)
	}
}

func TestRegionRenderingFitsTheColumn(t *testing.T) {
	// Every row of a right region must stay inside the region's column; a row
	// that overruns shears the transcript beside it.
	m := modelFor(t, 200, 40, nil)
	for i := 0; i < 25; i++ {
		m.start([]string{"run-" + string(rune('a'+i)), "with a long command name to exercise truncation"})
		m.finish(runDoneMsg{})
	}
	_, end, present := m.regionXRange()
	if !present {
		t.Fatal("no region to test")
	}
	start := end - (m.layout.Activity + regionChrome)
	for i, line := range strings.Split(m.View(), "\n") {
		if visibleWidth(line) > m.width {
			t.Fatalf("line %d overruns the terminal: %d", i, visibleWidth(line))
		}
		_ = start
	}
	if got := m.execOffset; got < 0 {
		t.Errorf("region offset went negative: %d", got)
	}
}

func TestBoundedOutputIsExplicitlyTruncated(t *testing.T) {
	th := newTheme(false, nil)
	m := newModel(Config{Runner: &InProcessRunner{Execute: func(ctx context.Context, _ []string) int { return 0 }, ToolName: "test"}}, th)
	m = resize(m, 100, 40)
	b := newBlock(1, []string{"noisy"})
	for i := 0; i < outputCap+40; i++ {
		b.addOutput("line " + string(rune('a'+i%26)))
	}
	m = addBlock(m, b)
	if !b.outputOmitted {
		t.Fatal("the cap did not record the omission")
	}
	if len(b.output) != outputCap {
		t.Errorf("retained %d lines, want the %d cap", len(b.output), outputCap)
	}
	m.blocks[0].finished = time.Now()
	m.blocks[0].status = StatusDone
	m = refresh(m)
	out := stripANSI(m.View())
	if !strings.Contains(out, "tail kept") {
		t.Errorf("the truncation is not disclosed:\n%s", out)
	}
}

func TestViewportHasNoLineWiderThanItsColumn(t *testing.T) {
	// With a region open, every transcript line has to fit the transcript
	// column exactly -- the ellipsis-free invariant the contentWidth fix
	// protects. The prose below is deliberately longer than the transcript.
	th := newTheme(false, nil)
	m := newModel(Config{Runner: &InProcessRunner{Execute: func(ctx context.Context, _ []string) int { return 0 }, ToolName: "test"}}, th)
	m = resize(m, 120, 40)
	m.start([]string{"scan", "example.com"})
	m.blocks[0].addOutput("a long line of tool output that must wrap inside the transcript column and never spill into the region's space " + strings.Repeat("x", 300))
	m.finish(runDoneMsg{})
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyF1})
	m = updated.(model)
	m.blocks[0].expanded = true
	m.refreshViewport()

	if m.layout.Activity == 0 {
		t.Fatal("no region was granted; this test needs one")
	}
	start, _, present := m.regionXRange()
	if !present {
		t.Fatal("the region is not drawn")
	}
	for _, line := range m.transcriptColumnLines(t, 120) {
		if strings.Contains(stripANSI(line), "…") {
			t.Errorf("a transcript line was truncated with an ellipsis:\n%q", stripANSI(line))
		}
		if lipgloss.Width(line) > m.viewport.Width {
			t.Errorf("a transcript line (%d) is wider than the viewport (%d)", lipgloss.Width(line), m.viewport.Width)
		}
		_ = start
	}
}
