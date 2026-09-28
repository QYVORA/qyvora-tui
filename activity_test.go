package tui

import (
	"strings"
	"testing"
	"time"
)

// The activity tests cover the two properties that make the region worth
// having: it counts what actually happened, and it does not claim a tool
// standardises on something it does not.

func ev(topic, level string, data map[string]any) Event {
	return Event{Event: topic, Level: level, Data: data}
}

func TestActivityCountsEveryEventWhateverItsTopic(t *testing.T) {
	// The fleet's topic distribution is bimodal: a few topics everyone emits,
	// then a long tail. A tail topic that the region ignores is a tool's whole
	// output disappearing from the interface, so every event is counted.
	a := newActivity()
	for _, topic := range []string{
		"scan.started", "scan.completed", "finding.discovered", "report.generated",
		"error", "warning", "info",
		"sekhmet.crash.detected", "mansa.handshake.captured", "jabari.device.attached",
		"something.a.tool.has.never.emitted.before",
	} {
		a.record(ev(topic, "info", nil))
	}
	if a.Total != 11 {
		t.Errorf("Total = %d, want 11", a.Total)
	}
	if a.ByLevel["info"] != 11 {
		t.Errorf("info count = %d, want 11", a.ByLevel["info"])
	}
}

func TestActivityTakesNoNoticeOfAMissingLevel(t *testing.T) {
	// A level is not a required field. A missing one is treated as info rather
	// than creating a "" bucket, so the count still adds up to the total.
	a := newActivity()
	a.record(ev("tool.event", "", nil))
	if a.Total != 1 {
		t.Fatalf("Total = %d", a.Total)
	}
	if a.ByLevel["info"] != 1 {
		t.Errorf("a level-less event was not counted as info: %v", a.ByLevel)
	}
}

func TestActivityCountsFindingsAndStages(t *testing.T) {
	a := newActivity()
	a.record(ev("stage.started", "info", map[string]any{"name": "recon"}))
	if a.CurrentStage != "recon" {
		t.Errorf("CurrentStage = %q, want recon", a.CurrentStage)
	}
	a.record(ev("finding.discovered", "info", nil))
	a.record(ev("finding.discovered", "info", nil))
	if a.Findings != 2 {
		t.Errorf("Findings = %d, want 2", a.Findings)
	}
	a.record(ev("stage.completed", "info", map[string]any{"name": "recon"}))
	if a.CurrentStage != "" {
		t.Errorf("CurrentStage = %q after completion, want empty", a.CurrentStage)
	}
	if a.LastStage != "recon" {
		t.Errorf("LastStage = %q, want recon", a.LastStage)
	}
}

func TestActivityReadsThePhaseNamingTheFleetAlsoUses(t *testing.T) {
	// Seven tools say stage.* and two say phase.*. Reading only one of them
	// would leave those tools with no progress shown, silently.
	a := newActivity()
	a.record(ev("phase.started", "info", map[string]any{"phase": "intel"}))
	if a.CurrentStage != "intel" {
		t.Errorf("phase.started gave CurrentStage = %q, want intel", a.CurrentStage)
	}
}

func TestActivityTailIsBounded(t *testing.T) {
	// A long run emits a great many events. The region shows the last few, so
	// the interface's memory does not grow with the length of the run.
	a := newActivity()
	for i := 0; i < 5000; i++ {
		a.record(ev("finding.discovered", "info", nil))
	}
	if len(a.Recent) != activityTail {
		t.Errorf("Recent holds %d entries, want the last %d", len(a.Recent), activityTail)
	}
	if a.Total != 5000 {
		t.Errorf("Total = %d, want 5000 -- the tail is bounded, the count is not", a.Total)
	}
	if a.Findings != 5000 {
		t.Errorf("Findings = %d, want 5000", a.Findings)
	}
}

func TestActivityResetBetweenRuns(t *testing.T) {
	// Each run gets a fresh Activity. Carrying counts across would report a new
	// scan as having inherited the previous one's work.
	first := newActivity()
	first.record(ev("finding.discovered", "info", nil))
	second := newActivity()
	if second.Total != 0 || second.Findings != 0 || len(second.Recent) != 0 {
		t.Errorf("a new Activity inherited state: %+v", second)
	}
}

func TestActivityNilIsSafe(t *testing.T) {
	// The region is reached from a render path that can run before any event
	// has arrived, so a nil Activity has to be inert rather than fatal.
	var a *Activity
	a.record(ev("scan.started", "info", nil))
	a.Note("ignored")
	if a.Elapsed() != 0 {
		t.Error("a nil Activity reported an elapsed time")
	}
	if a.hasErrors() {
		t.Error("a nil Activity reported errors")
	}
	if got := a.Region(newTheme(false, nil), 24); got == nil {
		t.Error("a nil Activity produced no region")
	}
}

func TestActivityHasErrors(t *testing.T) {
	for _, tc := range []struct {
		level string
		want  bool
	}{
		{"info", false}, {"warn", false}, {"warning", false},
		{"error", true}, {"critical", true}, {"fatal", true},
	} {
		a := newActivity()
		a.record(ev("tool.event", tc.level, nil))
		if got := a.hasErrors(); got != tc.want {
			t.Errorf("level %q: hasErrors() = %t, want %t", tc.level, got, tc.want)
		}
	}
}

func TestActivityRegionShowsCountsNotAPercentage(t *testing.T) {
	// A run that emits no progress data has no percentage. Rendering one would
	// be a fabrication, and a fabricated progress bar is worse than none.
	a := newActivity()
	a.record(ev("scan.started", "info", nil))
	a.record(ev("finding.discovered", "info", nil))
	got := stripANSI(a.Region(newTheme(false, nil), 24).Render(newTheme(false, nil)))
	if strings.Contains(got, "%") {
		t.Errorf("the activity region rendered a percentage: %q", got)
	}
	if !strings.Contains(got, "events") || !strings.Contains(got, "2") {
		t.Errorf("counts missing from the region: %q", got)
	}
	if !strings.Contains(got, "findings") || !strings.Contains(got, "1") {
		t.Errorf("the finding count is missing: %q", got)
	}
}

func TestActivityRegionIsEmptyBeforeAnythingHappens(t *testing.T) {
	got := stripANSI(newActivity().Region(newTheme(false, nil), 24).Render(newTheme(false, nil)))
	if !strings.Contains(got, "no activity") {
		t.Errorf("an untouched activity region = %q, want an explanation", got)
	}
}

func TestActivityRegionOmitsZeroLevels(t *testing.T) {
	// "critical 0" tells a reader nothing and pushes the interesting rows down.
	a := newActivity()
	a.record(ev("tool.event", "info", nil))
	a.record(ev("tool.event", "error", nil))
	got := stripANSI(a.Region(newTheme(false, nil), 24).Render(newTheme(false, nil)))
	if strings.Contains(got, "debug") || strings.Contains(got, "warn") {
		t.Errorf("a level with no events was listed: %q", got)
	}
	if !strings.Contains(got, "error") {
		t.Errorf("the error count is missing: %q", got)
	}
}

func TestActivityRegionShowsEveryLevelTheRunUsed(t *testing.T) {
	// A level the run actually emitted has to appear, including ones the TUI
	// does not recognise. A fixed list of level names would count a "critical"
	// or a "notice" event in the total and then leave it out of the display,
	// which is the one outcome this region exists to prevent.
	a := newActivity()
	for _, level := range []string{"critical", "error", "warning", "info", "debug", "notice", "severe-ish"} {
		a.record(ev("tool.event", level, nil))
	}
	th := newTheme(false, nil)
	got := stripANSI(a.Region(th, 24).Render(th))
	for _, level := range []string{"critical", "error", "warning", "info", "debug", "notice", "severe-ish"} {
		if !strings.Contains(got, level) {
			t.Errorf("a level the run used is not shown (%s):\n%s", level, got)
		}
	}
	// Urgent first: the ordering is the point of a region, since it is what an
	// operator reads before anything else.
	order := []string{"critical", "error", "warning"}
	at := -1
	for _, level := range order {
		i := strings.Index(got, level)
		if i < 0 {
			t.Fatalf("%s is missing:\n%s", level, got)
		}
		if i < at {
			t.Errorf("%s is shown after a less urgent level:\n%s", level, got)
		}
		at = i
	}
	// An unrecognised level is not promoted to an error it never claimed.
	if a.hasErrors() != true {
		t.Error("critical and error should count as errors")
	}
	bare := newActivity()
	bare.record(ev("tool.event", "severe-ish", nil))
	if bare.hasErrors() {
		t.Error("an unknown level was treated as an error")
	}
}

func TestActivityElapsedGrows(t *testing.T) {
	a := newActivity()
	a.Started = time.Now().Add(-3 * time.Second)
	if el := a.Elapsed(); el < 3*time.Second {
		t.Errorf("Elapsed = %v, want at least 3s", el)
	}
	bare := &Activity{}
	if bare.Elapsed() != 0 {
		t.Error("an Activity with no start time reported a duration")
	}
}

func TestModelActivityFollowsTheRun(t *testing.T) {
	// The wiring, end to end: events delivered to the model reach the activity
	// view, and a new command starts a new one.
	m := modelFor(t, 200, 40, nil)
	if m.activity != nil {
		t.Fatal("activity exists before any command ran")
	}
	if m.running {
		t.Fatal("the model reports running before a command")
	}
	// A command that emits nothing still yields an activity view, so a run
	// that produces no events is visibly empty rather than unexplained.
	m.start([]string{"noop"})
	if m.activity == nil {
		t.Fatal("a started run has no activity view")
	}
	m.Update(batchMsg{nil, evMsg(ev("scan.started", "info", nil))})
	if m.activity.Total != 1 {
		t.Errorf("an event was not counted: total %d", m.activity.Total)
	}
	m.Update(batchMsg{evMsg(ev("finding.discovered", "info", nil))})
	m.Update(batchMsg{evMsg(ev("tool.specific.thing", "warn", nil))})
	if m.activity.Total != 3 {
		t.Errorf("total = %d, want 3", m.activity.Total)
	}
	if m.activity.ByLevel["warn"] != 1 {
		t.Errorf("a tool-specific level was not counted: %v", m.activity.ByLevel)
	}
}

func TestModelActivityResetsOnTheNextCommand(t *testing.T) {
	m := modelFor(t, 200, 40, nil)
	m.start([]string{"one"})
	m.Update(batchMsg{evMsg(ev("finding.discovered", "info", nil))})
	first := m.activity
	if first.Total != 1 {
		t.Fatalf("first run counted %d", first.Total)
	}
	// start() refuses a second concurrent run, so the reset is exercised by
	// finishing the first.
	m.running = false
	m.start([]string{"two"})
	if m.activity == first {
		t.Error("the second run reused the first run's activity view")
	}
	if m.activity.Total != 0 {
		t.Errorf("the second run started with %d events", m.activity.Total)
	}
}
