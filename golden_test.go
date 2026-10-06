package tui

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// The golden frames are a review artefact as much as a regression test. A
// redesign is judged by looking at it, and eight stages of fixes have been
// verified one assertion at a time -- correct, and not once seen. The frames
// below are what the redesign actually looks like at 140 columns.
//
// Every frame here is asserted structurally as well as stored: a golden that
// only fails on a byte change cannot tell anyone that the interface is
// rectangular, that no row exceeds the terminal, or that the ladder is in order.
// Those are checked in this file so a golden diff is never the only signal.

const goldenCapsJSON = `[
 {"id":"probe.scan","name":"Scan","description":"Scan for wireless networks in range","framework":"probe","category":"scan","output_schema":["Finding","Artifact"]},
 {"id":"probe.assess","name":"Assess","description":"Run the full wireless assessment pipeline","framework":"probe","category":"scan","output_schema":["Finding"]},
 {"id":"probe.discover","name":"Discover","description":"Discover wireless interfaces on this host","framework":"probe","category":"radio","output_schema":["Artifact"]},
 {"id":"probe.analyze","name":"Analyze","description":"Analyze the latest session for security findings","framework":"probe","category":"analyze","output_schema":["Finding"]},
 {"id":"probe.report","name":"Report","description":"Write a signed report for the current session","framework":"probe","category":"export","output_schema":["Artifact"]},
 {"id":"probe.monitor","name":"Monitor","description":"Watch for new access points until stopped","framework":"probe","category":"monitor","output_schema":["Finding"]},
 {"id":"probe.identify","name":"Identify","description":"Fingerprint an access point from its beacons","framework":"probe","category":"radio","output_schema":["Finding"]},
 {"id":"probe.export","name":"Export","description":"Export captured frames as pcapng","framework":"probe","category":"export","output_schema":["Artifact"]}
]`

// goldenScene builds the state a real session passes through: a finished run
// with findings and artifacts above, and a scan in flight below. Every element
// of the redesign has to appear together somewhere, because the questions worth
// asking are about the whole frame -- does the running progress sit correctly
// beside two panels, does the newest finding stay above the fold -- and a scene
// with only one of them answers none of them.
func goldenScene(t *testing.T, w, h int) model {
	t.Helper()
	// Timestamps are relative to now rather than fixed. A running block's elapsed
	// time is measured against the wall clock, so a fixed date makes the frame
	// depend on the day it was generated: the stored row read "310731:39" and
	// would read something else tomorrow. That makes the golden fail on a
	// re-run with no change to the interface, which is how a golden gets
	// deleted instead of trusted.
	now := time.Now()
	at := func(s int) time.Time { return now.Add(-90*time.Second + time.Duration(s)*time.Second) }

	m := modelFor(t, w, h, normalize(t, "probe", goldenCapsJSON))
	m.theme = newTheme(true, nil)
	m.theme.Depth = DepthTrueColor
	m.cfg.Banner = ToolBanner("probe", "wireless assessment")
	m.showCapabilities = true
	m.input.SetValue("probe scan --band 2.4")

	// A finished run, collapsed, with the three severities and an artifact. The
	// finding tally in the summary row and the region count both come from here.
	done := &block{
		id: 1, command: "probe analyze --last", started: at(0), finished: at(37), status: StatusDone,
		findings: []Finding{
			{Title: "WPA2 uses a 4-character PSK", Severity: "critical", Target: "eduroam", Detail: "PSK entropy exhausted; offline dictionary recovery is trivial.", At: at(31)},
			{Title: "Management frame protection disabled", Severity: "high", Target: "NETGEAR-42", Detail: "deauth frames accepted without a valid MIC.", At: at(33)},
			{Title: "Beacon interval at the 102.4ms default", Severity: "medium", Target: "linksys-guest", Detail: "suggests the access point is unmodified.", At: at(35)},
		},
		artifacts: []Artifact{{Name: "analysis.sarif", Kind: "sarif", Size: "84.2 kB", At: at(37)}},
	}

	// A run in flight. The progress bar has to survive beside two panels at
	// 140 columns, and the transcript column is what decides whether it does.
	//
	// The elapsed time of a running block is measured against the wall clock and
	// there is no injectable clock, so its second count changes on every run. The
	// block is given a start time in the past and the golden's own comparison
	// normalises the volatile field, which is the only part of the frame that
	// cannot be pinned. Asserting on a wall-clock reading would mean the stored
	// frame was wrong within a minute of being written.
	live := &block{
		id: 2, command: "probe scan --band 2.4", started: now.Add(-47 * time.Second), status: StatusRunning,
		progress: progressLine{Current: 812_004, Total: 1_048_576, Percent: 77.46,
			Message: "scanning channels 1-11", HasBar: true},
		rows: []eventRow{
			{At: at(41), Level: "info", Type: "radio.change", Label: "channel change",
				Detail: "wlan0 moved from channel 6 to channel 11"},
			{At: at(44), Level: "info", Type: "probe.beacon", Label: "beacon seen",
				Detail: "NETGEAR-42 on 11 at -58 dBm, WPA2-Enterprise"},
		},
		output: []string{
			"# probe 0.8.0 · wireless assessment",
			"iface       wlan0 (monitor mode, channel 11)",
			"driver      iwlwifi 6.35.5-1",
			"beacons     812004 / 1048576",
		},
	}

	prev := &block{id: 3, command: "probe discover", started: at(38), finished: at(39), status: StatusFailed, exitCode: 2,
		output: []string{"no interfaces in monitor mode: set one with `probe radio set wlan0 monitor`"}}

	m = addBlock(m, done)
	m = addBlock(m, prev)
	m = addBlock(m, live)
	m = refresh(m)

	// The running block's live state is tracked beside the blocks, not inside
	// them, because it belongs to the session rather than to any one run. Without
	// it the executions entry reads "starting…" beside a transcript that already
	// reports the stage, which is the one combination that looks like a bug in
	// the panel rather than in the fixture.
	m.activity = &Activity{
		Total: 1_284, Findings: 2, Started: live.started,
		ByLevel:      map[string]int{"info": 1_276, "warn": 6, "error": 2},
		Recent:       []string{"beacon seen", "channel change", "probe.beacon NETGEAR-42 on 11 at -58 dBm"},
		CurrentStage: "scanning channels 1-11",
	}
	return m
}

// withColorProfile runs fn with the process's colour profile pinned.
func withColorProfile(t *testing.T, p termenv.Profile, fn func()) {
	t.Helper()
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	lipgloss.SetColorProfile(p)
	fn()
}

func TestGoldenReviewFrames(t *testing.T) {
	const w, h = 140, 44
	withColorProfile(t, termenv.TrueColor, func() {
		m := goldenScene(t, w, h)
		frame := m.View()
		golden(t, "wide-findings-and-live", frame)
		// Structural assertions: the golden answers "what does it look like",
		// these answer "is it a frame at all".
		rows := strings.Split(frame, "\n")
		if len(rows) != h {
			t.Errorf("rows %d, want %d", len(rows), h)
		}
		for i, r := range rows {
			if got := lipgloss.Width(stripANSI(r)); got != w {
				t.Errorf("row %d is %d columns, want %d", i, got, w)
			}
		}
		// The ground reaches the frame's own margins, so no row is left showing
		// the terminal's background at either edge.
		inset := leadingSGR(m.theme.Inset.Render("x"))
		for i, r := range rows {
			if !strings.Contains(r, inset) {
				t.Errorf("row %d is off the frame's ground", i)
			}
		}
	})
}

func TestGoldenNarrowLosesRegions(t *testing.T) {
	// The narrow frame is stored too. A redesign that only ever looks right at
	// 140 columns is a redesign that has not been finished, and the panels are
	// the first thing to go when the terminal cannot afford them.
	withColorProfile(t, termenv.TrueColor, func() {
		m := goldenScene(t, 64, 30)
		golden(t, "narrow", m.View())
		rows := strings.Split(m.View(), "\n")
		for i, r := range rows {
			if got := lipgloss.Width(stripANSI(r)); got != 64 {
				t.Errorf("row %d is %d columns, want 64", i, got)
			}
		}
		if strings.Contains(stripANSI(m.View()), "EXECUTIONS") {
			t.Error("executions panel still drawn at 64 columns")
		}
	})
}

func TestGoldenNoColorIsTheSameInterface(t *testing.T) {
	// The interface must survive NO_COLOR as the same interface, not as a
	// degraded one. This frame is stored so a change to the plain layout is
	// visible in a diff rather than only in a reviewer's terminal.
	withColorProfile(t, termenv.Ascii, func() {
		m := goldenScene(t, 140, 44)
		m.theme = newTheme(false, nil)
		m.theme.Depth = DepthTrueColor
		frame := m.View()
		if strings.Contains(frame, "\x1b") {
			t.Fatalf("escape codes with colour off: %q", frame)
		}
		golden(t, "wide-no-color", frame)
		rows := strings.Split(frame, "\n")
		for i, r := range rows {
			if got := lipgloss.Width(stripANSI(r)); got != 140 {
				t.Errorf("row %d is %d columns, want 140", i, got)
			}
		}
	})
}

// golden writes a frame for review and fails on a change to it.
//
// The stored frame is only compared when one exists, and the file is written on
// the first run. A test that cannot be reviewed until someone has looked for the
// file is a test that gets deleted, and the frames here exist to be looked at.
func golden(t *testing.T, name, frame string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name+".txt")
	want, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("reading golden %s: %v", path, err)
	}
	if len(want) == 0 {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("creating %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(frame), 0o644); err != nil {
			t.Fatalf("writing golden %s: %v", path, err)
		}
		t.Logf("wrote golden %s (%d bytes) -- review it, then commit", path, len(frame))
		return
	}
	// UPDATE_GOLDEN=1 is the deliberate way to accept a change. A golden that
	// rewrites itself on any mismatch is a golden that can never fail.
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(path, []byte(frame), 0o644); err != nil {
			t.Fatalf("updating golden %s: %v", path, err)
		}
		t.Logf("updated golden %s", path)
		return
	}
	if normaliseGolden(string(want)) != normaliseGolden(frame) {
		t.Errorf("%s differs from the golden.\nReview, then: UPDATE_GOLDEN=1 go test -run TestGolden .\n%s",
			name, firstDifference(string(want), frame))
	}
}

// normaliseGolden replaces the readings that cannot be pinned, so a stored frame
// is compared on everything that is actually decided by the interface.
//
// A running execution's elapsed time is time.Since on the real clock, so it
// differs on every run. Everything else in the frame -- widths, fills, wrapping,
// truncation, the surface ladder -- is a function of the state and is compared
// exactly. One field being excused is defensible; the alternative is a golden
// that fails every run and is deleted on the first bad morning.
func normaliseGolden(frame string) string {
	// A running block shows "▸ ⠿ <command> MM:SS". The duration is the only
	// volatile token in the frame.
	return durationToken.ReplaceAllString(frame, "${1}<elapsed>${2}")
}

var durationToken = regexp.MustCompile(`(\x1b\[[0-9;]*m)?[0-9]{1,4}:[0-9]{2}(\x1b\[0m)?`)

func firstDifference(want, got string) string {
	a, b := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := range max(len(a), len(b)) {
		x, y := pick(a, i), pick(b, i)
		if x != y {
			return strconv.Itoa(i) + ": want " + quote(x) + ", got " + quote(y)
		}
	}
	return "identical"
}

// quote renders a row for a failure message. The frames are escape-heavy and the
// visible text is what a reviewer reads, so a diff has to quote that and not the
// SGR soup around it.
func quote(s string) string {
	if s == "" {
		return "<none>"
	}
	return strconv.Quote(stripANSI(s))
}

// The header declared fewer rows than it drew.
//
// This was a real frame-height fault, not a cosmetic one, and it is here as its
// own test because the golden would only ever report it as a changed byte. The
// status pill does not fit beside a sixty-column wordmark, so it is given a row
// of its own -- but headerHeight counted only the art. chromeHeight charges the
// transcript for what the header says it occupies, so the viewport was handed
// rows that were never there and View wrote one row more than the terminal had.
// The terminal scrolls, and the row that leaves is the composer's input row: the
// frame grew downwards by exactly the row the operator was typing into.
//
// Every configuration is checked, because the fault only appears when the pill
// goes below, and the widths where it still sits beside are the widths where the
// frame is accidentally correct.
func TestFrameIsExactlyAsTallAsTheTerminalAtEveryWidth(t *testing.T) {
	withColorProfile(t, termenv.TrueColor, func() {
		b := ToolBanner("amanirenas", "wireless security platform")
		for _, w := range []int{12, 20, 30, 40, 60, 80, 100, 140, 200} {
			for _, h := range []int{10, 12, 16, 24, 40, 44} {
				m := modelFor(t, w, h, nil)
				m.theme = newTheme(true, nil)
				m.theme.Depth = DepthTrueColor
				m.cfg.Banner = b
				m = addBlock(m, &block{id: 1, command: "probe scan", started: time.Now(), status: StatusRunning})
				m = refresh(m)
				m = resize(m, w, h)

				rows := strings.Split(m.View(), "\n")
				if len(rows) != h {
					t.Errorf("w=%d h=%d: frame is %d rows", w, h, len(rows))
				}
				// A row of zero columns in the middle of a full-width frame is
				// the same fault seen from the other side: the reserved strip
				// drawn as an empty string rather than as a blank row.
				for i, r := range rows {
					if got := lipgloss.Width(stripANSI(r)); got != w {
						t.Errorf("w=%d h=%d: row %d is %d columns, want %d", w, h, i, got, w)
					}
				}
			}
		}
	})
}
