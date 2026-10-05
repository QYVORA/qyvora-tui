package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Design-contract tests for the visual work. These assert the properties the
// redesign is responsible for, so a later change that reintroduces the old
// shape fails here rather than being noticed on screen.

func TestDesignContractFindingsUseTheAvailableWidth(t *testing.T) {
	m := newModel(Config{Title: "T"}, newTheme(false, nil))
	m.width = 160
	m.viewport.Width = 160
	m.viewport.Height = 20
	title, target := m.findingColumns()
	if title <= 34 {
		t.Errorf("title column = %d: the old hardcoded cap of 34 is back, so a wide "+
			"terminal truncates titles mid-word while most of it sits empty", title)
	}
	if target < minFindingTarget {
		t.Errorf("target column = %d, want at least %d", target, minFindingTarget)
	}
	if title+target >= m.contentWidth() {
		t.Errorf("columns (%d + %d) do not fit the transcript width %d", title, target, m.contentWidth())
	}
}

func TestDesignContractNarrowTerminalDropsAnUnreadableTarget(t *testing.T) {
	m := newModel(Config{Title: "T"}, newTheme(false, nil))
	m.width = 40
	m.viewport.Width = 40
	m.viewport.Height = 20
	title, target := m.findingColumns()
	if target != 0 {
		t.Errorf("target column = %d at 40 columns: an address squeezed into a handful "+
			"of cells identifies nothing, so it should be dropped", target)
	}
	if title < minFindingTitle {
		t.Errorf("title column = %d, want at least %d", title, minFindingTitle)
	}
}

func TestDesignContractComposerHintSurvivesTheFullWidthInput(t *testing.T) {
	// The input widget pads itself to the whole terminal, so measuring the
	// rendered line reports the line as always full and the hints were dropped on
	// every terminal. This is the regression guard for that.
	for _, w := range []int{80, 100, 140} {
		m := newModel(Config{Title: "T"}, newTheme(false, nil))
		m.width, m.height = w, 30
		m.applyLayout(w, 30)
		m.input.SetValue("scan example.com")
		got := stripANSI(m.renderComposer())
		if !strings.Contains(got, "F1") {
			t.Errorf("width %d: composer hint missing, so the keys are undiscoverable:\n%s", w, got)
		}
		if !strings.Contains(got, "ctrl+e") {
			t.Errorf("width %d: composer hint missing the export key:\n%s", w, got)
		}
	}
}

func TestDesignContractHintNamesTheKeysThatActuallyWork(t *testing.T) {
	// F2 moves focus to the executions region; enter is what expands. A hint
	// claiming F2 expands on its own sends people pressing a key that does
	// nothing visible.
	m := newModel(Config{Title: "T"}, newTheme(false, nil))
	m.width, m.height = 120, 30
	m.applyLayout(120, 30)
	b := &block{id: 1, command: "scan", output: []string{"one", "two"}}

	m.regionFocus = false
	unfocused := stripANSI(strings.Join(m.renderOutput(b, false), "\n"))
	if !strings.Contains(unfocused, "F2 focus") {
		t.Errorf("unfocused hint does not name the focus step: %q", unfocused)
	}
	m.regionFocus = true
	focused := stripANSI(strings.Join(m.renderOutput(b, false), "\n"))
	if strings.Contains(focused, "F2") {
		t.Errorf("focused hint still names F2, which is no longer needed: %q", focused)
	}
	if !strings.Contains(focused, "enter") {
		t.Errorf("focused hint does not name the key that expands: %q", focused)
	}
}

func TestDesignContractChromeFitsTheTerminal(t *testing.T) {
	// An open form stands in for the composer but can be taller than it. If the
	// chrome is not measured, the form pushes its own fields off the screen.
	for _, tc := range []struct{ rows, cols, fields int }{
		{40, 120, 1}, {40, 120, 8}, {24, 80, 5}, {12, 40, 3},
	} {
		m := newModel(Config{Title: "T"}, newTheme(false, nil))
		m.width, m.height = tc.cols, tc.rows
		m.applyLayout(tc.cols, tc.rows)
		m.blocks = []*block{{id: 1, command: "scan", status: StatusDone}}
		var params []Parameter
		for i := 0; i < tc.fields; i++ {
			params = append(params, Parameter{Name: fmt.Sprintf("p%d", i)})
		}
		m.form = OpenForm(Capability{ID: "c", Name: "C", Input: params})
		if m.form == nil {
			continue
		}
		m.applyLayout(tc.cols, tc.rows)
		m.refreshViewport()
		if got := len(strings.Split(m.View(), "\n")); got > tc.rows {
			t.Errorf("%dx%d with %d fields rendered %d rows: the interface is taller "+
				"than the terminal", tc.cols, tc.rows, tc.fields, got)
		}
	}
}

func TestDesignContractGutterGroupsABlockBody(t *testing.T) {
	m := newModel(Config{Title: "T"}, newTheme(false, nil))
	m.width, m.height = 100, 30
	m.applyLayout(100, 30)
	b := &block{
		id: 1, command: "scan", status: StatusDone,
		started: time.Now().Add(-time.Second), finished: time.Now(),
		findings: []Finding{{Title: "weak tls", Severity: "LOW", Target: "h"}},
	}
	lines := m.gutter(m.renderBlockBody(b))
	for i, l := range lines {
		if strings.TrimSpace(stripANSI(l)) == "" {
			continue
		}
		if !strings.HasPrefix(stripANSI(l), " "+gutterRail+" ") {
			t.Errorf("body line %d is not inside the gutter: %q", i, stripANSI(l))
		}
	}
}

func TestDesignContractSeveritySpellsTheWordOut(t *testing.T) {
	th := newTheme(false, nil)
	for sev, want := range map[string]string{
		"CRITICAL": "CRITICAL", "crit": "CRITICAL", "high": "HIGH",
		"medium": "MEDIUM", "moderate": "MEDIUM", "med": "MEDIUM",
		"low": "LOW", "info": "INFO", "nonsense": "INFO", "": "INFO",
	} {
		if got := strings.TrimSpace(th.severityTag(sev)); got != want {
			t.Errorf("severityTag(%q) = %q, want %q", sev, got, want)
		}
	}
	// Every canonical severity occupies the same column, which is what lines the
	// list up.
	if w := lipgloss.Width(th.severityTag("INFO")); w != severityColWidth {
		t.Errorf("severity column = %d, want %d", w, severityColWidth)
	}
}

func TestDesignContractWorstSeverityColoursTheTally(t *testing.T) {
	fs := []Finding{{Severity: "LOW"}, {Severity: "CRITICAL"}, {Severity: "MEDIUM"}}
	if got := worstSeverity(fs); got != "CRITICAL" {
		t.Errorf("worstSeverity = %q, want CRITICAL", got)
	}
	if got := worstSeverity([]Finding{{Severity: "LOW"}, {Severity: "MEDIUM"}}); got != "MEDIUM" {
		t.Errorf("worstSeverity = %q, want MEDIUM", got)
	}
	if got := worstSeverity(nil); got != "" {
		t.Errorf("worstSeverity(nil) = %q, want empty", got)
	}
	// An unrecognised label must not be promoted above a real severity.
	if got := worstSeverity([]Finding{{Severity: "bogus"}}); got != "" {
		t.Errorf("worstSeverity invented a severity: %q", got)
	}
}

func TestDesignContractSummaryCountsFindingsOnce(t *testing.T) {
	m := newModel(Config{Title: "T"}, newTheme(false, nil))
	m.width, m.height = 100, 30
	m.applyLayout(100, 30)
	b := &block{
		id: 1, command: "scan", status: StatusDone,
		started: time.Now().Add(-3 * time.Second), finished: time.Now(), known: 4,
		findings:  []Finding{{Severity: "HIGH", Title: "t"}},
		artifacts: []Artifact{{Name: "r.json"}},
	}
	got := stripANSI(m.renderSummary(b))
	if n := strings.Count(got, "finding"); n != 1 {
		t.Errorf("summary mentions findings %d times, want once: %q", n, got)
	}
	for _, want := range []string{"completed", "1 finding", "1 artifact", "4 events"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary missing %q: %q", want, got)
		}
	}
}

func TestDesignContractNoFindingsIsNotGreenOnAFailedRun(t *testing.T) {
	// lipgloss reads its colour profile from stdout, so under `go test` every
	// style renders as plain text and an assertion about colour would pass for
	// the wrong reason. Forcing the profile is what makes this test mean what it
	// says; it is restored afterwards so no other test inherits it.
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	th := newTheme(true, nil)
	if !th.Color {
		t.Skip("colour is off in this environment, so there is nothing to assert")
	}
	// Compare against the escape sequence lipgloss actually emits rather than
	// the palette's hex: a hex colour is rendered as numeric RGB, so looking for
	// the hex would pass for the wrong reason.
	probe := th.Success.Render("x")
	prefix := probe[:strings.Index(probe, "x")]
	if prefix == "" {
		t.Fatal("no colour was emitted even with the profile forced; the assertion below is void")
	}

	m := newModel(Config{Title: "T"}, th)
	m.width, m.height = 100, 30
	m.applyLayout(100, 30)

	failed := m.renderSummary(&block{id: 1, command: "s", status: StatusFailed, finished: time.Now()})
	if strings.Contains(failed, prefix) && strings.Contains(failed, "no findings") {
		t.Error("a failed run's \"no findings\" is painted green: a run that died before " +
			"looking is not a clean bill of health")
	}
	done := m.renderSummary(&block{id: 1, command: "s", status: StatusDone, finished: time.Now()})
	if !strings.Contains(done, prefix) {
		t.Error("a clean run's \"no findings\" lost its positive colour")
	}
}
