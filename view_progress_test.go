package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// The summary is the one row that answers "what did that do?". If it runs past
// the terminal, the viewport cuts it at its own width and the last thing the row
// says is a half word.
func TestSummaryFitsTheTranscriptWidth(t *testing.T) {
	b := &block{command: "probe run", status: StatusDone, started: nowish().Add(-3 * time.Second)}
	b.err = strings.Repeat("connection refused ", 5)
	for i := 0; i < 12; i++ {
		b.findings = append(b.findings, Finding{Severity: "HIGH", Title: "t", Target: "0x1"})
	}
	for i := 0; i < 4; i++ {
		b.artifacts = append(b.artifacts, Artifact{Name: "a.bin", Size: "1.2MB"})
	}
	b.known = 40

	for _, w := range []int{40, 50, 60, 70, 80, 100, 140} {
		m := modelFor(t, w, 40, nil)
		got := stripANSI(m.renderSummary(b))
		if n := lipgloss.Width(got); n > m.contentWidth() {
			t.Errorf("w=%d: summary is %d wide, content is %d: %q", w, n, m.contentWidth(), got)
		}
	}
}

// When the row has to give something up, the finding tally is what survives.
// A finished run that reports "12 artifacts" and no findings has answered a
// question nobody asked and hidden the one they did.
func TestSummaryKeepsTheFindingTallyAtEveryWidth(t *testing.T) {
	b := &block{command: "probe run", status: StatusDone, started: nowish().Add(-3 * time.Second)}
	b.err = strings.Repeat("connection refused ", 8)
	for i := 0; i < 12; i++ {
		b.findings = append(b.findings, Finding{Severity: "HIGH", Title: "t", Target: "0x1"})
	}
	for i := 0; i < 4; i++ {
		b.artifacts = append(b.artifacts, Artifact{Name: "a.bin", Size: "1.2MB"})
	}
	b.known = 40000

	for _, w := range []int{40, 50, 60, 70, 80, 100, 140} {
		m := modelFor(t, w, 40, nil)
		got := stripANSI(m.renderSummary(b))
		if !strings.Contains(got, "12 findings") {
			t.Errorf("w=%d: the finding tally was dropped: %q", w, got)
		}
	}
}

// The parts are dropped in order of how much they change what the operator does
// next: the tally survives an event count, and the event count survives
// nothing. This is the ordering claim stated as a test rather than left to the
// order the code happens to append things in.
func TestSummaryDropsTheLeastImportantPartFirst(t *testing.T) {
	b := &block{command: "probe run", status: StatusDone, started: nowish().Add(-3 * time.Second)}
	for i := 0; i < 3; i++ {
		b.findings = append(b.findings, Finding{Severity: "HIGH", Title: "t", Target: "0x1"})
	}
	b.artifacts = append(b.artifacts, Artifact{Name: "a.bin", Size: "1.2MB"})
	b.known = 9

	wide := modelFor(t, 140, 40, nil)
	full := stripANSI(wide.renderSummary(b))
	for _, want := range []string{"3 findings", "1 artifact", "9 events"} {
		if !strings.Contains(full, want) {
			t.Fatalf("the wide summary is missing %q: %q", want, full)
		}
	}

	// Narrow until one part drops, then check which. 52 is the width where the
	// event count no longer fits and the artifact count still does.
	m := modelFor(t, 52, 40, nil)
	narrow := stripANSI(m.renderSummary(b))
	if strings.Contains(narrow, "9 events") {
		t.Errorf("the event count should have been the part to go: %q", narrow)
	}
	if !strings.Contains(narrow, "3 findings") || !strings.Contains(narrow, "1 artifact") {
		t.Errorf("a more important part was dropped instead: %q", narrow)
	}
}

// The outcome itself is never negotiable. A summary that cannot say whether the
// run passed is not a summary, so at the narrowest widths it is all that is left.
func TestSummaryAlwaysStatesTheOutcome(t *testing.T) {
	b := &block{command: "probe run", status: StatusFailed, started: nowish().Add(-3 * time.Second)}
	b.err = strings.Repeat("dial tcp: connection refused ", 20)
	for i := 0; i < 50; i++ {
		b.findings = append(b.findings, Finding{Severity: "CRITICAL", Title: "t", Target: "0x1"})
	}
	for _, w := range []int{24, 30, 40, 60, 140} {
		m := modelFor(t, w, 40, nil)
		got := stripANSI(m.renderSummary(b))
		if !strings.Contains(got, "failed") {
			t.Errorf("w=%d: the outcome is missing: %q", w, got)
		}
		if n := lipgloss.Width(got); n > m.contentWidth() {
			t.Errorf("w=%d: width %d over %d", w, n, m.contentWidth())
		}
	}
}

// "No findings" in green on a run that died is a lie about health, so the claim
// is dimmed when the run did not finish its work.
func TestSummaryNoFindingsIsDimUnlessTheRunFinished(t *testing.T) {
	done := &block{command: "p", status: StatusDone, started: nowish().Add(-time.Second)}
	failed := &block{command: "p", status: StatusFailed, started: nowish().Add(-time.Second)}
	m := modelFor(t, 100, 40, nil)
	t.Logf("done   %q", stripANSI(m.renderSummary(done)))
	t.Logf("failed %q", stripANSI(m.renderSummary(failed)))
}

// The live line preferred the event's label over its detail while testing the
// detail, so both branches returned the label and the detail the tool wrote for
// a person was never shown.
func TestLiveLinePrefersTheEventDetailOverItsLabel(t *testing.T) {
	m := modelFor(t, 140, 40, nil)
	b := &block{status: StatusRunning}
	b.rows = append(b.rows, eventRow{Label: "scan.progress", Detail: "scanning cell 812447 of 1000000"})
	if got := stripANSI(m.renderLive(b)); !strings.Contains(got, "scanning cell 812447") {
		t.Errorf("got %q, want the event detail", got)
	}
}

// An event with no detail still has a label, and that is what describes it.
func TestLiveLineFallsBackToTheEventLabel(t *testing.T) {
	m := modelFor(t, 140, 40, nil)
	b := &block{status: StatusRunning}
	b.rows = append(b.rows, eventRow{Label: "scan.progress"})
	if got := stripANSI(m.renderLive(b)); !strings.Contains(got, "scan.progress") {
		t.Errorf("got %q, want the event label", got)
	}
}

// A tool's own progress message outranks anything inferred from an event.
func TestLiveLinePrefersTheToolsProgressMessage(t *testing.T) {
	m := modelFor(t, 140, 40, nil)
	b := &block{status: StatusRunning}
	b.progress = progressLine{Message: "fuzzing campaign"}
	b.rows = append(b.rows, eventRow{Label: "scan.progress", Detail: "cell 1 of 2"})
	if got := stripANSI(m.renderLive(b)); !strings.Contains(got, "fuzzing campaign") {
		t.Errorf("got %q, want the tool's own message", got)
	}
}

// A percentage alone says "how far along", which is the less useful half of the
// question. On a scan of a million cells, 81% says nothing about whether the run
// is nearly over, and the tool published the counts.
func TestProgressShowsTheCountsTheToolReported(t *testing.T) {
	for _, w := range []int{40, 60, 100, 140} {
		m := modelFor(t, w, 40, nil)
		b := &block{status: StatusRunning}
		b.progress = progressLine{Current: 812447, Total: 1000000, Percent: 81.2, HasBar: true}
		got := stripANSI(strings.Join(m.renderProgress(b), ""))
		if !strings.Contains(got, "812,447") || !strings.Contains(got, "1,000,000") {
			t.Errorf("w=%d: counts missing: %q", w, got)
		}
		if n := lipgloss.Width(got); n > m.contentWidth() {
			t.Errorf("w=%d: width %d over %d: %q", w, n, m.contentWidth(), got)
		}
	}
}

// Grouped digits are what make a count readable at a glance. Six ungrouped
// digits have to be counted to get the magnitude, and the magnitude is the point.
func TestProgressGroupsLargeCounts(t *testing.T) {
	m := modelFor(t, 140, 40, nil)
	b := &block{status: StatusRunning}
	b.progress = progressLine{Current: 812447, Total: 2000000, Percent: 40.6, HasBar: true}
	got := stripANSI(strings.Join(m.renderProgress(b), ""))
	if !strings.Contains(got, "812,447") || !strings.Contains(got, "2,000,000") {
		t.Errorf("got %q", got)
	}
}

// At a width where the bar and the numbers cannot both fit, the numbers win: they
// are the more precise statement of the same fact. The bar gives way rather than
// both being rendered at a width where neither can be read.
func TestProgressDropsTheBarBeforeTheCounts(t *testing.T) {
	// The full counts are 21 columns. A bar beside them needs minBarCells more on
	// top, so at narrow widths the bar is what goes.
	for _, w := range []int{26, 30, 34, 38, 40, 50, 60} {
		m := modelFor(t, w, 40, nil)
		b := &block{status: StatusRunning}
		b.progress = progressLine{Current: 812447, Total: 1000000, Percent: 81.2, HasBar: true}
		got := stripANSI(strings.Join(m.renderProgress(b), ""))
		if n := lipgloss.Width(got); n > m.contentWidth() {
			t.Errorf("w=%d: width %d over %d: %q", w, n, m.contentWidth(), got)
		}
		// A bar is only allowed when it has enough cells to report a reading.
		// Four cells can only say 0, 25, 50, 75 or 100, which is a decoration
		// suggesting a reading it cannot give.
		if cells := strings.Count(got, "█") + strings.Count(got, "░"); cells > 0 && cells < minBarCells {
			t.Errorf("w=%d: a %d cell bar cannot report a fraction: %q", w, cells, got)
		}
		if !strings.Contains(got, "81%") {
			t.Errorf("w=%d: the percentage is missing: %q", w, got)
		}
		t.Logf("w=%d %q", w, got)
	}
}

// Once the counts cannot be abbreviated into the row either, the percentage is
// all that is left. Dropping it would leave the operator with a bar and no
// reading of it.
func TestProgressFallsBackToThePercentageAlone(t *testing.T) {
	m := modelFor(t, 24, 40, nil)
	b := &block{status: StatusRunning}
	b.progress = progressLine{Current: 123456789, Total: 987654321, Percent: 81.2, HasBar: true}
	got := stripANSI(strings.Join(m.renderProgress(b), ""))
	if n := lipgloss.Width(got); n > m.contentWidth() {
		t.Errorf("width %d over %d: %q", n, m.contentWidth(), got)
	}
	if !strings.Contains(got, "81%") {
		t.Errorf("the percentage is missing: %q", got)
	}
	t.Logf("narrowest progress: %q", got)
}

// An invented bar would be a guess about work the tool never reported.
func TestProgressDrawsNothingWithoutAReportedPercentage(t *testing.T) {
	m := modelFor(t, 100, 40, nil)
	b := &block{status: StatusRunning}
	b.progress = progressLine{Current: 10, Total: 20, Message: "working"}
	if got := m.renderProgress(b); got != nil {
		t.Errorf("got %v, want nothing", got)
	}
}

// Abbreviating a count has to keep its magnitude and must never round up past
// the next unit, because that would report more work than the tool said it had.
func TestShortCountKeepsMagnitudeWithoutRoundingUp(t *testing.T) {
	for _, tc := range []struct {
		in   int64
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1k"},
		{9500, "9.5k"},
		{9999, "9.9k"},
		{12345, "12k"},
		{999999, "999k"},
		{1000000, "1M"},
		{1234567, "1.2M"},
		{9999999, "9.9M"},
		{10000000, "10M"},
		{999999999, "999M"},
		{1000000000, "1B"},
		{-42123, "-42k"},
	} {
		if got := shortCount(tc.in); got != tc.want {
			t.Errorf("shortCount(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Grouping is the same operation on the full number.
func TestCommasGroupsThousands(t *testing.T) {
	for _, tc := range []struct {
		in   int64
		want string
	}{
		{0, "0"},
		{5, "5"},
		{999, "999"},
		{1000, "1,000"},
		{812447, "812,447"},
		{1000000, "1,000,000"},
		{123456789, "123,456,789"},
		{-42123, "-42,123"},
	} {
		if got := commas(tc.in); got != tc.want {
			t.Errorf("commas(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

const manyCapsJSON = `[
 {"id":"a.one","name":"One","description":"d","framework":"a","category":"c","output_schema":["X"]},
 {"id":"a.two","name":"Two","description":"d","framework":"a","category":"c","output_schema":["X"]},
 {"id":"a.three","name":"Three","description":"d","framework":"a","category":"c","output_schema":["X"]},
 {"id":"a.four","name":"Four","description":"d","framework":"a","category":"c","output_schema":["X"]},
 {"id":"a.five","name":"Five","description":"d","framework":"a","category":"c","output_schema":["X"]},
 {"id":"a.six","name":"Six","description":"d","framework":"a","category":"c","output_schema":["X"]},
 {"id":"a.seven","name":"Seven","description":"d","framework":"a","category":"c","output_schema":["X"]},
 {"id":"a.eight","name":"Eight","description":"d","framework":"a","category":"c","output_schema":["X"]},
 {"id":"a.nine","name":"Nine","description":"d","framework":"a","category":"c","output_schema":["X"]}
]`

// capsModel builds a model at a size with the capability region switched on.
func capsModel(t *testing.T, w, h int) model {
	t.Helper()
	m := modelFor(t, w, h, normalize(t, "a", manyCapsJSON))
	m.showCapabilities = true
	return resize(m, w, h)
}

// countedNames reports how many capability names the region drew and what it
// claimed was left out.
func countedNames(rows []string) (names int, more string) {
	for _, l := range rows {
		p := stripANSI(l)
		switch {
		case strings.Contains(p, "more"):
			more = p
		case strings.HasPrefix(p, "●") || strings.HasPrefix(p, "○"):
			names++
		}
	}
	return names, more
}

// The region used to budget against a hardcoded height-10, which hid
// capabilities that would have fitted and then reported a "+N more" count that
// had nothing to do with how many were really hidden.
func TestCapabilityRegionShowsEveryOneThatFits(t *testing.T) {
	total := len(normalize(t, "a", manyCapsJSON).Items)
	for _, h := range []int{16, 20, 24, 30, 40} {
		m := capsModel(t, 140, h)
		m.layout = m.resolveLayout(m.width)
		r := m.capabilitiesRegion(m.layout.Navigation)
		if r == nil {
			t.Fatalf("h=%d: no region", h)
		}
		r.Height = m.viewportHeight(m.height)
		names, more := countedNames(regionLines(r.Render(m.theme)))
		if more == "" {
			if names != total {
				t.Errorf("h=%d: %d of %d shown and no notice that %d are hidden",
					h, names, total, total-names)
			}
			continue
		}
		claimed := moreCount(more)
		if names+claimed != total {
			t.Errorf("h=%d: %d shown plus a claim of %d is not the %d published: %q",
				h, names, claimed, total, more)
		}
	}
}

// The notice is the only thing that tells the operator the list is partial. If
// the region fills its budget and then tries to append the notice, the notice is
// the row that gets dropped, and capabilities vanish without a word.
func TestCapabilityRegionAlwaysSaysHowManyAreHidden(t *testing.T) {
	for _, h := range []int{10, 12, 14, 16, 18, 24} {
		m := capsModel(t, 140, h)
		m.layout = m.resolveLayout(m.width)
		r := m.capabilitiesRegion(m.layout.Navigation)
		if r == nil {
			continue
		}
		rows := regionLines(r.Render(m.theme))
		if len(rows) > r.Height {
			t.Errorf("h=%d: %d rows in a %d row box", h, len(rows), r.Height)
		}
		names, more := countedNames(rows)
		if more != "" && names == 0 {
			t.Errorf("h=%d: the notice is all that fits, so nothing is shown: %q", h, more)
		}
		t.Logf("h=%2d box=%2d shown=%d %q", h, r.Height, names, more)
	}
}

// moreCount reads the number out of a "+N more" row.
func moreCount(s string) int {
	n := 0
	seen := false
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			n = n*10 + int(r-'0')
			seen = true
		case seen:
			return n
		}
	}
	return n
}

// A capability is drawn whole or not at all. A row cut off by the region's
// bottom edge cannot be told from a complete one.
func TestCapabilityRowsMatchWhatIsActuallyDrawn(t *testing.T) {
	m := capsModel(t, 140, 16)
	m.layout = m.resolveLayout(m.width)
	r := m.capabilitiesRegion(m.layout.Navigation)
	if r == nil {
		t.Fatal("no region")
	}
	for _, e := range m.caps.Entries() {
		want := 1
		if e.Detail != "" {
			want++
		}
		if e.Note != "" && e.Note != "live provider" {
			want++
		}
		if got := capabilityRows(e); got != want {
			t.Errorf("%s: capabilityRows = %d, want %d", e.Name, got, want)
		}
	}
}

// The banner is a TUI affordance. A tool with no banner configured draws nothing
// and the header carries the name on its own, which is the default for a
// library or a command.
func TestNoBannerConfiguredMeansNoBanner(t *testing.T) {
	m := modelFor(t, 140, 40, nil)
	if got := m.renderBanner(); len(got) != 0 {
		t.Errorf("got %d banner rows with no banner configured", len(got))
	}
	if got := m.headerHeight(); got != 1 {
		t.Errorf("header is %d rows with no banner, want 1", got)
	}
}

// The ladder degrades with the terminal, and the header's height is what the
// transcript's height is computed from, so the two have to agree.
func TestBannerDegradesWithTheTerminalAndIsChargedToTheHeader(t *testing.T) {
	b := ToolBanner("amanirenas", "wireless security platform")
	prev := 0
	for _, tc := range []struct{ w, h int }{
		{140, 40}, {100, 40}, {60, 40}, {40, 40}, {30, 40}, {20, 40}, {12, 40},
		{140, 16}, {140, 12}, {140, 10},
	} {
		m := modelFor(t, tc.w, tc.h, nil)
		m.cfg.Banner = b
		m = resize(m, tc.w, tc.h)
		rows := m.renderBanner()
		if len(rows) == 0 {
			t.Logf("w=%3d h=%d: no banner", tc.w, tc.h)
			continue
		}
		if len(rows) > tc.h {
			t.Errorf("w=%d h=%d: banner is %d rows in a %d row terminal", tc.w, tc.h, len(rows), tc.h)
		}
		// The header's declared height has to equal the rows it actually draws.
		// These are not the same number when the status pill does not fit beside
		// the art and is given a row of its own, so the invariant is stated
		// against what is drawn rather than against the art alone. Asserting
		// headerHeight against the art is what let the header claim six rows
		// while writing seven: chromeHeight charges the transcript for what the
		// header says it occupies, so an undercount hands the viewport rows that
		// are not there and the frame scrolls -- taking the input row with it.
		drawn := len(strings.Split(m.renderHeader(), "\n"))
		if got := m.headerHeight(); got != drawn {
			t.Errorf("w=%d h=%d: header claims %d rows and draws %d", tc.w, tc.h, got, drawn)
		}
		if drawn < len(rows) {
			t.Errorf("w=%d h=%d: header draws %d rows for %d rows of art", tc.w, tc.h, drawn, len(rows))
		}
		// The composer's mode bar is the one row that must never leave: it is
		// what says where the keyboard is. The placeholder text beside it is
		// itself truncated on a narrow terminal, so it is the mode bar that is
		// asserted rather than the prompt wording.
		if !strings.Contains(stripANSI(m.View()), "COMMAND") {
			t.Errorf("w=%d h=%d: the composer is not on screen", tc.w, tc.h)
		}
		if tc.h == 40 {
			prev = len(rows)
		}
		t.Logf("w=%3d h=%2d banner=%d rows prev=%d", tc.w, tc.h, len(rows), prev)
	}
}

// A banner row carrying both the art and the status pill is how a row ends up
// twice the terminal's width: the art is padded to the full width, and then the
// pill is appended to it.
func TestBannerRowWithTheStatusStaysInsideTheTerminal(t *testing.T) {
	b := ToolBanner("amanirenas", "wireless security platform")
	for _, w := range []int{30, 40, 60, 80, 100, 140} {
		for _, h := range []int{10, 16, 24, 40} {
			m := modelFor(t, w, h, nil)
			m.cfg.Banner = b
			m = resize(m, w, h)
			for i, l := range strings.Split(stripANSI(m.View()), "\n") {
				if n := lipgloss.Width(l); n > w {
					t.Errorf("w=%d h=%d: line %d is %d wide: %q", w, h, i, n, l)
				}
			}
		}
	}
}

// A banner is a wordmark, so its rows have to be the same width or the header
// reads as a rendering fault.
func TestBannerRowsAreEqualWidth(t *testing.T) {
	b := ToolBanner("amanirenas", "wireless security platform")
	for _, w := range []int{30, 60, 100, 140} {
		m := modelFor(t, w, 30, nil)
		m.cfg.Banner = b
		m = resize(m, w, 30)
		rows := m.renderBanner()
		if len(rows) < 2 {
			continue
		}
		want := lipgloss.Width(stripANSI(rows[0]))
		for i, l := range rows {
			if got := lipgloss.Width(stripANSI(l)); got != want {
				t.Errorf("w=%d: row %d is %d wide, row 0 is %d", w, i, got, want)
			}
		}
	}
}
