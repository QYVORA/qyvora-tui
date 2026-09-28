package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The shell tests are about the interface as an operator meets it: a resized
// terminal, a tool with a registry, a tool without one, and a run in flight.

func TestShellDrawsNoRegionsWhenThereIsNothingToShow(t *testing.T) {
	// The default case, and the one that matters most for the two tools with
	// no capability registry: full-width transcript, no empty column.
	for _, w := range []int{80, 120, 200} {
		m := modelFor(t, w, 40, nil)
		out := stripANSI(m.View())
		if !strings.Contains(out, "ready") {
			t.Errorf("width %d: header missing:\n%s", w, out)
		}
		if strings.Contains(out, "CAPABILITIES") || strings.Contains(out, "ACTIVITY") {
			t.Errorf("width %d: an empty region was drawn:\n%s", w, out)
		}
	}
}

func TestShellDrawsCapabilitiesWhenTheToolHasThem(t *testing.T) {
	caps := normalize(t, "sekhmet", familyAJSON)
	for _, w := range []int{200, 140} {
		m := modelFor(t, w, 40, caps)
		m.showCapabilities = true
		out := stripANSI(m.View())
		if !strings.Contains(out, "CAPABILITIES") {
			t.Errorf("width %d: the capability region is missing:\n%s", w, out)
		}
		// Registry names, not ids: the name is what an operator recognises. It
		// is truncated to the region's width, which is the correct trade in a
		// 24-column column, so the assertion is on the leading part of the name
		// rather than the whole string.
		if !strings.Contains(out, "Profile targ") {
			t.Errorf("width %d: a capability name is missing:\n%s", w, out)
		}
		// The risk grade is the part that changes a decision, so it has to
		// survive the truncation. It is dropped before the name is.
		if !strings.Contains(out, "S1") {
			t.Errorf("width %d: the risk grade was lost to truncation:\n%s", w, out)
		}
	}
}

func TestShellHidesCapabilitiesUntilAsked(t *testing.T) {
	// The region is a reference, not the interface. It is hidden until F1 so it
	// does not compete with the session for attention.
	caps := normalize(t, "sekhmet", familyAJSON)
	m := modelFor(t, 200, 40, caps)
	if strings.Contains(stripANSI(m.View()), "CAPABILITIES") {
		t.Error("the capability region was drawn without being asked for")
	}
	if !strings.Contains(stripANSI(m.hint()), "F1") {
		t.Errorf("F1 is not discoverable: %q", m.hint())
	}
	// model is a value type, so an update is returned rather than applied in
	// place. Taking the returned model is what a real Update loop does.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyF1})
	m = updated.(model)
	if !m.showCapabilities {
		t.Fatal("F1 did not toggle the region")
	}
	if !strings.Contains(stripANSI(m.View()), "CAPABILITIES") {
		t.Error("the region is still hidden after F1")
	}
	if !strings.Contains(stripANSI(m.hint()), "F1") {
		t.Errorf("the hint no longer offers to close the region: %q", m.hint())
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyF1})
	m = updated.(model)
	if m.showCapabilities {
		t.Error("a second F1 did not close the region")
	}
}

func TestShellKeepsANarrowTerminalSingleColumn(t *testing.T) {
	// Below the breakpoint the regions are not drawn even when there is
	// something to draw. This is the narrow-terminal requirement: the existing
	// single column, not a squeezed version of the wide one.
	caps := normalize(t, "sekhmet", familyAJSON)
	for _, w := range []int{20, 40, 59} {
		m := modelFor(t, w, 40, caps)
		out := stripANSI(m.View())
		if strings.Contains(out, "CAPABILITIES") {
			t.Errorf("width %d: a region was drawn on a narrow terminal:\n%s", w, out)
		}
		if m.layout.Mode != ModeCompact {
			t.Errorf("width %d: mode = %v, want compact", w, m.layout.Mode)
		}
	}
}

func TestShellDrawsActivityWhileRunning(t *testing.T) {
	// A run in flight is the case the right-hand column exists for.
	m := modelFor(t, 200, 40, nil)
	m.start([]string{"run"})
	m.Update(batchMsg{
		evMsg(ev("scan.started", "info", nil)),
		evMsg(ev("finding.discovered", "info", nil)),
		evMsg(ev("tool.specific.thing", "error", nil)),
	})
	out := stripANSI(m.View())
	if !strings.Contains(out, "ACTIVITY") {
		t.Errorf("no activity region while running:\n%s", out)
	}
	// The error is the thing an operator is looking for, so it leads.
	if !strings.Contains(out, "error") {
		t.Errorf("the error count is missing:\n%s", out)
	}
	if m.layout.Activity == 0 {
		t.Error("the layout granted no activity width while running")
	}
}

func TestShellChargesNoColumnsForARegionThatIsNotDrawn(t *testing.T) {
	// A region can exist and still draw nothing: too short to render, or wider
	// than its own content. Charging the transcript for a panel that is not on
	// screen is how the columns end up over-running the terminal, and the
	// over-run shows up as a truncated transcript rather than as an obvious
	// failure.
	caps := normalize(t, "sekhmet", familyAJSON)
	for _, w := range []int{40, 60, 100, 200} {
		m := modelFor(t, w, 40, caps)
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyF1})
		m = updated.(model)
		m.start([]string{"scan", "--sim"})
		for i, line := range strings.Split(m.View(), "\n") {
			if got := visibleWidth(line); got > w {
				t.Errorf("width %d: line %d is %d wide:\n%q", w, i, got, line)
			}
		}
	}
	// A zero width means the layout granted nothing, and a region built anyway
	// would be drawn into no columns at all.
	m := modelFor(t, 60, 40, caps)
	m.start([]string{"scan"})
	if r := m.activityRegion(0); r != nil {
		t.Error("a region was built at zero width")
	}
	if r := m.capabilitiesRegion(0); r != nil {
		t.Error("a capability region was built at zero width")
	}
}

func TestShellSurvivesAnUnknownEventType(t *testing.T) {
	// The point of the region: a topic the TUI has never seen is still counted
	// and still shown. This is the regression that would return if the region
	// started switching on topic names.
	m := modelFor(t, 200, 40, nil)
	m.start([]string{"run"})
	weird := "a.topic.no.tool.has.emitted.and.nobody.planned.for"
	m.Update(batchMsg{evMsg(ev(weird, "warning", nil))})
	if m.activity.Total != 1 {
		t.Errorf("an unknown topic was dropped: total %d", m.activity.Total)
	}
	if m.activity.ByLevel["warning"] != 1 {
		t.Errorf("an unknown topic's level was dropped: %v", m.activity.ByLevel)
	}
	// And it did not take the interface down.
	if out := stripANSI(m.View()); out == "" {
		t.Error("the interface rendered nothing after an unknown event")
	}
	// The live line shows the current state, so an unknown topic reaches the
	// transcript while the run is in flight. An unknown topic is shown as its own
	// label -- the topic's words in title case -- rather than dropped or replaced
	// by a generic one, because the label is the only description of it there is.
	if out := stripANSI(m.View()); !strings.Contains(out, "A Topic No Tool Has Emitted") {
		t.Errorf("the unknown event did not reach the transcript:\n%s", out)
	}

	// The event is retained on the block, not just aggregated: when the run
	// finishes, the raw stream with its levels becomes readable under Ctrl+O.
	// A tool-specific topic that was only counted would be unreadable after the
	// fact, which is exactly when someone goes looking for it.
	m.finish(runDoneMsg{})
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m = updated.(model)
	if !m.showEvents {
		t.Fatal("Ctrl+O did not reveal the raw event log")
	}
	if got := stripANSI(strings.Join(m.transcriptLines(), "\n")); !strings.Contains(got, "WARN") {
		t.Errorf("the raw event's level was lost:\n%s", got)
	}
	if got := stripANSI(strings.Join(m.transcriptLines(), "\n")); !strings.Contains(got, "A Topic No Tool Has Emitted") {
		t.Errorf("the raw event is not in the retained stream:\n%s", got)
	}
}

func TestShellResizesWithoutLosingTheSession(t *testing.T) {
	caps := normalize(t, "sekhmet", familyAJSON)
	m := modelFor(t, 200, 40, caps)
	m.start([]string{"first"})
	m.Update(batchMsg{evMsg(ev("scan.started", "info", nil))})
	// Resize across every breakpoint in both directions.
	for _, w := range []int{40, 80, 60, 200, 120, 20, 140} {
		updated, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: 40})
		m = updated.(model)
		if m.layout.Width != w {
			t.Errorf("width %d: layout.Width = %d", w, m.layout.Width)
		}
		out := stripANSI(m.View())
		if !strings.Contains(out, "first") {
			t.Errorf("width %d: the session was lost on resize:\n%s", w, out)
		}
		if m.activity == nil || m.activity.Total != 1 {
			t.Errorf("width %d: the activity view was lost on resize", w)
		}
	}
}

func TestShellSurvivesAnAbsurdResize(t *testing.T) {
	// Terminals report nonsense sizes during teardown, and a model that divides
	// by one of them panics at the worst moment.
	m := modelFor(t, 200, 40, nil)
	m.start([]string{"run"})
	for _, s := range []tea.WindowSizeMsg{{Width: 0, Height: 0}, {Width: -1, Height: -1}, {Width: 1, Height: 1}, {Width: 500, Height: 2}} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic on size %dx%d: %v", s.Width, s.Height, r)
				}
			}()
			updated, _ := m.Update(s)
			m = updated.(model)
			_ = m.View()
		}()
	}
}

func TestShellAdoptsCapabilitiesArrivingLate(t *testing.T) {
	// A tool may publish capabilities that change; the interface adopts them
	// without a restart rather than showing what it read at startup.
	m := modelFor(t, 200, 40, nil)
	if m.caps != nil {
		t.Fatal("a model with no registry has capabilities")
	}
	updated, _ := m.Update(capabilitiesMsg{caps: normalize(t, "sekhmet", familyAJSON)})
	m = updated.(model)
	if m.caps == nil {
		t.Fatal("late capabilities were not adopted")
	}
	// Adopting a registry is not the same as showing it: the region still waits
	// for F1, or a tool that publishes capabilities would open a column on every
	// start whether the operator wanted one or not.
	if m.showCapabilities {
		t.Error("adopting capabilities opened the region without being asked")
	}
	if strings.Contains(stripANSI(m.View()), "CAPABILITIES") {
		t.Errorf("the region is visible before F1:\n%s", m.View())
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyF1})
	m = updated.(model)
	if !strings.Contains(stripANSI(m.View()), "CAPABILITIES") {
		t.Errorf("the adopted registry is not shown after F1:\n%s", m.View())
	}
}

func TestShellRendersInPlainMode(t *testing.T) {
	// The NO_COLOR path has to produce the whole interface, not a stripped-down
	// one. A region drawn with a colour-only branch would be missing here.
	m := modelFor(t, 200, 40, normalize(t, "sekhmet", familyAJSON))
	m.showCapabilities = true
	m.start([]string{"run"})
	m.Update(batchMsg{evMsg(ev("finding.discovered", "info", nil))})
	out := m.View()
	if strings.Contains(out, "\x1b") {
		t.Error("the plain theme emitted an escape sequence")
	}
	for _, want := range []string{"CAPABILITIES", "ACTIVITY", "Profile target baseline", "run"} {
		if !strings.Contains(out, want) {
			t.Errorf("plain mode lost %q:\n%s", want, out)
		}
	}
}

func TestShellRegionsDoNotEllipsiseTheTranscript(t *testing.T) {
	// The bug this guards against is a mismatch between what the layout charges
	// the transcript for and what the composer actually writes: the layout
	// accounted for a region's own columns but not for the rule and the space
	// beside it, so the transcript was built a few columns too wide and every
	// line came back clamped with an ellipsis in the space the region had
	// already vacated.
	//
	// The isolated layout and composer tests cannot see this, because each is
	// handed consistent numbers. Only the assembled shell disagrees with itself.
	// F1 rather than the field directly: showing the region resizes the
	// viewport, and a test that skipped that step would be measuring a stale
	// layout rather than the one an operator gets.
	caps := normalize(t, "sekhmet", familyAJSON)
	for _, w := range []int{60, 80, 100, 119, 120, 121, 160, 240} {
		for _, running := range []bool{false, true} {
			m := modelFor(t, w, 40, caps)
			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyF1})
			m = updated.(model)
			if running {
				m.start([]string{"scan", "--sim"})
				m.Update(batchMsg{
					evMsg(ev("scan.started", "info", nil)),
					evMsg(ev("finding.discovered", "info", nil)),
				})
			}
			// Only the transcript is asserted. A region is meant to truncate: a
			// capability name that does not fit 20 columns has to be cut, and the
			// panel is the right place for that to happen. The transcript is the
			// opposite case -- it is supposed to reflow, never to be cut, and an
			// ellipsis there means the columns disagree about the width.
			for i, line := range m.transcriptColumnLines(t, w) {
				if strings.Contains(stripANSI(line), "…") {
					t.Errorf("width %d running=%t: transcript line %d was truncated:\n%q", w, running, i, stripANSI(line))
				}
			}
		}
	}
}

// transcriptColumnLines returns the transcript's slice of each rendered line.
//
// The column is located from the layout rather than by searching the text, so a
// capability whose name happens to contain the characters being looked for
// cannot be mistaken for the transcript, and it is bounded by the transcript's
// own width so a region on the right is not read as part of it.
func (m model) transcriptColumnLines(t *testing.T, width int) []string {
	t.Helper()
	l := m.layout
	start := 0
	if l.Navigation > 0 && m.capabilitiesRegion(l.Navigation) != nil {
		start = l.Navigation + regionChrome + regionGap
	}
	out := make([]string, 0, 8)
	for _, line := range strings.Split(m.View(), "\n") {
		plain := []rune(stripANSI(line))
		if start >= len(plain) {
			out = append(out, "")
			continue
		}
		end := start + l.Transcript
		if end > len(plain) {
			end = len(plain)
		}
		out = append(out, string(plain[start:end]))
	}
	return out
}

func TestShellWritesNoLineWiderThanTheTerminal(t *testing.T) {
	// The invariant the whole layout rests on. A single over-wide line shears
	// every column beside it and is the most visible rendering bug there is.
	caps := normalize(t, "sekhmet", familyAJSON)
	for _, w := range []int{20, 40, 60, 80, 100, 120, 160, 240} {
		for _, running := range []bool{false, true} {
			m := modelFor(t, w, 40, caps)
			m.showCapabilities = true
			if running {
				m.start([]string{"a-command-with-some-arguments", "--and-another", "value=with-a-long-tail"})
				m.Update(batchMsg{
					evMsg(ev("scan.started", "info", nil)),
					evMsg(ev("finding.discovered", "info", nil)),
					evMsg(ev("a.very.long.tool.specific.event.topic.name", "error", nil)),
				})
			}
			for i, line := range strings.Split(m.View(), "\n") {
				if got := visibleWidth(line); got > w {
					t.Errorf("width %d running=%t: line %d is %d wide:\n%q", w, running, i, got, line)
				}
			}
		}
	}
}

// visibleWidth measures a rendered line, ignoring the escape sequences colour
// adds. Comparing len() would report a styled line as far wider than the
// terminal and make every assertion here useless.
func visibleWidth(s string) int { return lipgloss.Width(s) }
