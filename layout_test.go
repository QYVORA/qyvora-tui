package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// The layout tests pin the breakpoints the audit measured, because the failure
// mode is a terminal 20 columns wide rendering two side regions that each need
// 24: no error, no panic, just a transcript nobody can read.

func TestLayoutBreakpoints(t *testing.T) {
	for _, tc := range []struct {
		name       string
		width      int
		wantMode   LayoutMode
		wantRegion bool
	}{
		// Below the compact breakpoint nothing is offered a region, whatever it
		// has to show. A 40-column terminal has no room for two columns and
		// never will.
		{name: "tiny", width: 20, wantMode: ModeCompact},
		{name: "just below compact", width: CompactWidth - 1, wantMode: ModeCompact},
		// 60 is where a single region becomes possible.
		{name: "at compact boundary", width: CompactWidth, wantMode: ModeAdaptive, wantRegion: true},
		{name: "mid", width: 100, wantMode: ModeAdaptive, wantRegion: true},
		// Beyond 120 both sides fit.
		{name: "at wide boundary", width: WideWidth, wantMode: ModeWide, wantRegion: true},
		{name: "very wide", width: 200, wantMode: ModeWide, wantRegion: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := LayoutFor(tc.width, 40, LayoutOptions{Navigation: true, Activity: true})
			if l.Mode != tc.wantMode {
				t.Errorf("Mode = %v, want %v at width %d", l.Mode, tc.wantMode, tc.width)
			}
			if l.ShowRegions != tc.wantRegion {
				t.Errorf("ShowRegions = %t, want %t at width %d", l.ShowRegions, tc.wantRegion, tc.width)
			}
		})
	}
}

func TestLayoutOffersNoRegionWithoutContent(t *testing.T) {
	// A tool with no capabilities and nothing running gets the whole width, on
	// any terminal. This is the rule that keeps an empty column from appearing
	// beside a working session.
	for _, w := range []int{40, 80, 200} {
		l := LayoutFor(w, 40, LayoutOptions{})
		if l.ShowRegions {
			t.Errorf("width %d: regions offered with nothing to show", w)
		}
		if l.Transcript != w {
			t.Errorf("width %d: transcript = %d, want the full %d", w, l.Transcript, w)
		}
	}
}

func TestLayoutTranscriptAlwaysFits(t *testing.T) {
	// The invariant the whole component set rests on: transcript + regions +
	// separators is never wider than the terminal, and the transcript is never
	// zero, at any width and any combination of offered regions.
	//
	// It is also an equality, not an inequality, and that is the point. An
	// inequality passes while the transcript is handed fewer columns than the
	// layout promised it, and the composer then clamps every line to make them
	// fit -- ellipsising the transcript in exactly the space the region vacated.
	for _, w := range []int{1, 20, 59, 60, 61, 80, 100, 120, 121, 200, 400} {
		for _, opts := range []LayoutOptions{
			{}, {Navigation: true}, {Activity: true},
			{Navigation: true, Activity: true},
		} {
			l := LayoutFor(w, 40, opts)
			if l.Transcript < 1 {
				t.Errorf("width %d opts %+v: transcript = %d, want at least 1", w, opts, l.Transcript)
			}
			used := l.Transcript + regionColumns(l.Navigation, l.Activity)
			if used > w {
				t.Errorf("width %d opts %+v: columns total %d, over the terminal width", w, opts, used)
			}
			if used != w {
				t.Errorf("width %d opts %+v: columns total %d, want the terminal exactly", w, opts, used)
			}
			if l.ShowRegions && l.Navigation == 0 && l.Activity == 0 {
				t.Errorf("width %d opts %+v: ShowRegions with no region", w, opts)
			}
		}
	}
}

func TestLayoutIgnoresNonsensicalSizes(t *testing.T) {
	// A zero or negative size reaches the layout on a terminal that reports one
	// during teardown. It must not produce a negative column width for a region
	// to render into.
	for _, w := range []int{0, -1, -100} {
		l := LayoutFor(w, 40, LayoutOptions{Navigation: true, Activity: true})
		if l.Transcript < 1 {
			t.Errorf("width %d: transcript = %d", w, l.Transcript)
		}
		if l.Navigation < 0 || l.Activity < 0 {
			t.Errorf("width %d: negative region width (%d, %d)", w, l.Navigation, l.Activity)
		}
	}
}

func TestLayoutHeightGatesRegions(t *testing.T) {
	// A region needs vertical room to be worth anything. On a very short
	// terminal it is dropped rather than squeezed: a 3-line activity region
	// next to a 5-line transcript helps nobody.
	l := LayoutFor(200, 3, LayoutOptions{Navigation: true, Activity: true})
	if l.ShowRegions {
		t.Error("regions offered on a 3-line terminal")
	}
	if l.Transcript != 200 {
		t.Errorf("transcript = %d, want the full 200", l.Transcript)
	}
}

func TestLayoutIsStableForAStableSize(t *testing.T) {
	// The transcript width must not vary between renders, or a region and the
	// viewport disagree and the columns shear.
	first := LayoutFor(100, 40, LayoutOptions{Navigation: true, Activity: true})
	for i := 0; i < 5; i++ {
		l := LayoutFor(100, 40, LayoutOptions{Navigation: true, Activity: true})
		if l != first {
			t.Fatalf("layout is not deterministic: %+v then %+v", first, l)
		}
	}
}

func TestComposeRegionsAlignsColumns(t *testing.T) {
	// The failure this guards against is the classic one: a viewport returns
	// short lines, the regions have their own heights, and without padding the
	// right-hand column slides up as the transcript scrolls.
	mid := "aaa\nb\ncccc"
	left := "L1\nL2\nL3\nL4"
	right := "R1\nR2"
	for _, w := range []int{40, 60, 100} {
		lw, rw := 16, 16
		out := strings.Split(composeRegions(mid, w-lw-rw-2, left, lw, right, rw, w), "\n")
		if len(out) != 4 {
			t.Fatalf("width %d: got %d lines, want 4", w, len(out))
		}
		for i, line := range out {
			if got := lipgloss.Width(line); got > w {
				t.Errorf("width %d line %d is %d wide, over the terminal", w, i, got)
			}
			if got := lipgloss.Width(line); got != w {
				t.Errorf("width %d line %d is %d wide, want the terminal exactly", w, i, got)
			}
		}
		// The columns are in the same place on every line, or the right-hand
		// column slides as the transcript scrolls.
		for i, line := range out {
			if i >= 2 {
				continue
			}
			plain := stripANSI(line)
			if idx := strings.Index(plain, "R"); idx < lw+1 {
				t.Errorf("width %d: line %d has the right column at %d: %q", w, i, idx, plain)
			}
		}
		// A line with no right-region content is padded, not skipped, so the
		// column above it is not orphaned.
		if !strings.HasPrefix(stripANSI(out[0]), "L1") {
			t.Errorf("width %d: line 0 = %q", w, stripANSI(out[0]))
		}
	}
}

func TestComposeRegionsHandlesUnevenSides(t *testing.T) {
	// One region only, and regions taller than the transcript. Both directions
	// occur: a long capability list beside a short command, and a long run
	// beside a short registry.
	if got := composeRegions("x", 30, "", 0, "R1\nR2", 10, 40); !strings.Contains(got, "R2") {
		t.Errorf("right region shorter than the transcript was lost: %q", got)
	}
	if got := composeRegions("x\ny", 30, "L1", 10, "", 0, 40); !strings.Contains(got, "y") {
		t.Errorf("transcript lost when only a left region was present: %q", got)
	}
	if got := composeRegions("x", 30, "L1\nL2\nL3", 10, "", 0, 40); !strings.Contains(got, "L3") {
		t.Errorf("left region taller than the transcript was lost: %q", got)
	}
	if got := composeRegions("x", 40, "", 0, "", 0, 40); !strings.Contains(got, "x") {
		t.Errorf("the transcript was lost with no regions at all: %q", got)
	}
}

func TestComposeRegionsDoesNotEllipsiseTheTranscript(t *testing.T) {
	// The regression that made this function take explicit widths: measuring the
	// middle column gave the full terminal width, so every transcript line was
	// clamped and came out with an ellipsis where the region now was.
	w, lw, rw := 100, 20, 20
	mid := strings.Repeat("x", w-lw-rw-2)
	out := stripANSI(composeRegions(mid, w-lw-rw-2, "L1", lw, "R1", rw, w))
	if strings.Contains(out, "…") {
		t.Errorf("the transcript was ellipsised:\n%s", out)
	}
	if lipgloss.Width(out) != w {
		t.Errorf("line is %d wide, want %d", lipgloss.Width(out), w)
	}
}

// TestSidebarWinsBelowWideWidth pins the one asymmetric grant in the layout. The
// capability registry is reference material behind F1; executions are the live
// session. Below WideWidth the transcript goes to the one that cannot wait.
func TestSidebarWinsBelowWideWidth(t *testing.T) {
	both := LayoutOptions{Navigation: true, Activity: true}
	for _, w := range []int{CompactWidth, 80, 100, WideWidth - 1} {
		l := LayoutFor(w, 40, both)
		if l.Navigation != 0 {
			t.Errorf("width %d: navigation = %d, want 0 below WideWidth", w, l.Navigation)
		}
		if l.Activity == 0 {
			t.Errorf("width %d: no executions region, but activity was offered", w)
		}
	}
	for _, w := range []int{WideWidth, WideWidth + 1, 200} {
		l := LayoutFor(w, 40, both)
		if l.Navigation == 0 || l.Activity == 0 {
			t.Errorf("width %d: navigation = %d, activity = %d; want both at WideWidth and above", w, l.Navigation, l.Activity)
		}
	}
}

// TestRegistryFillsTheWideTerminalAlone covers the case where there is no run in
// flight: a wide terminal with nothing to execute falls back to the registry
// rather than to an empty column.
func TestRegistryFillsTheWideTerminalAlone(t *testing.T) {
	l := LayoutFor(200, 40, LayoutOptions{Navigation: true})
	if l.Navigation == 0 {
		t.Fatal("no navigation region on a wide terminal with nothing to execute")
	}
	if l.Activity != 0 {
		t.Errorf("activity = %d with no executions to show", l.Activity)
	}
	if used := l.Transcript + regionColumns(l.Navigation, l.Activity); used != 200 {
		t.Errorf("columns total %d, want the terminal exactly", used)
	}
}

// TestCompactWidthIsDerivedFromTheParts guards the breakpoint against the region
// floor moving under it. A hand-written CompactWidth stops meaning anything the
// moment minRegionWidth changes, and the failure is a layout that grants a panel
// it then has to take back.
func TestCompactWidthIsDerivedFromTheParts(t *testing.T) {
	want := minRegionWidth + regionChrome + regionGap + minTranscriptWidth
	if CompactWidth != want {
		t.Errorf("CompactWidth = %d, want %d from the region floor and the transcript minimum", CompactWidth, want)
	}
	// Exactly at the breakpoint the narrowest region and the narrowest
	// transcript must both fit. One column either side must not both.
	l := LayoutFor(CompactWidth, 40, LayoutOptions{Activity: true})
	if l.Activity == 0 {
		t.Errorf("no region at CompactWidth %d", CompactWidth)
	}
	if got := l.Transcript; got < minTranscriptWidth {
		t.Errorf("transcript = %d at CompactWidth, below the %d minimum", got, minTranscriptWidth)
	}
	if l := LayoutFor(CompactWidth-1, 40, LayoutOptions{Activity: true}); l.ShowRegions {
		t.Errorf("width %d granted a region below CompactWidth", CompactWidth-1)
	}
}

// TestRegionWidthFollowsTheShare pins the width formula across its whole range:
// a quarter of the terminal less a margin, clamped at both ends.
func TestRegionWidthFollowsTheShare(t *testing.T) {
	for _, tc := range []struct{ width, want int }{
		{CompactWidth, minRegionWidth},
		{116, minRegionWidth}, // (116-4)/4 = 28, the floor exactly
		{120, 29},             // the share is live from here
		{140, 34},
		{148, 36},             // (148-4)/4 = 36, the ceiling exactly
		{240, maxRegionWidth}, // clamped
	} {
		if got := regionWidth(tc.width); got != tc.want {
			t.Errorf("regionWidth(%d) = %d, want %d", tc.width, got, tc.want)
		}
	}
}

// Both regions are drawn at the same width so the transcript does not change
// size as a panel opens and closes.
func TestBothRegionsShareOneWidth(t *testing.T) {
	for _, w := range []int{CompactWidth, 100, WideWidth, 200} {
		l := LayoutFor(w, 40, LayoutOptions{Navigation: true, Activity: true})
		if l.Navigation == 0 || l.Activity == 0 {
			continue
		}
		if l.Navigation != l.Activity {
			t.Errorf("width %d: navigation %d, activity %d; want them equal", w, l.Navigation, l.Activity)
		}
	}
}
