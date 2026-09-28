package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestRegionRendersTitleAndRows(t *testing.T) {
	r := NewRegion("CAPABILITIES", 24)
	r.Add("one")
	r.Add("two")
	got := stripANSI(r.Render(newTheme(false, nil)))
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want title + rule + 2 rows:\n%s", len(lines), got)
	}
	if lines[0] != "CAPABILITIES" {
		t.Errorf("title = %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "─") {
		t.Errorf("rule = %q", lines[1])
	}
	if lines[2] != "one" || lines[3] != "two" {
		t.Errorf("rows = %q, %q", lines[2], lines[3])
	}
}

func TestRegionNilAndEmptyAreSafe(t *testing.T) {
	// A region that is absent and a region with nothing in it are both normal
	// states: a tool with no registry, and a run that has emitted nothing yet.
	// Neither may panic, because both are reached from a render path.
	var nilRegion *Region
	if got := nilRegion.Render(newTheme(false, nil)); got != "" {
		t.Errorf("nil region rendered %q", got)
	}
	if nilRegion.Len() != 0 {
		t.Errorf("nil region reported rows")
	}
	nilRegion.Add("x")
	nilRegion.SetRows([]string{"x"})

	empty := NewRegion("ACTIVITY", 24)
	got := stripANSI(empty.Render(newTheme(false, nil)))
	if !strings.Contains(got, "ACTIVITY") {
		t.Errorf("an empty region dropped its title: %q", got)
	}
	if len(empty.Rows) != 0 {
		t.Error("an empty region grew rows")
	}
}

func TestRegionEmptyTextIsShownWhenSet(t *testing.T) {
	// "none published" is more useful than a blank column, and blank is what an
	// unset Empty field produces.
	withText := NewRegion("CAPABILITIES", 24)
	withText.Empty = "none published"
	if got := stripANSI(withText.Render(newTheme(false, nil))); !strings.Contains(got, "none published") {
		t.Errorf("Empty text not shown: %q", got)
	}
	without := NewRegion("CAPABILITIES", 24)
	if got := stripANSI(without.Render(newTheme(false, nil))); strings.Contains(got, "  \n") {
		t.Errorf("an unset Empty left trailing blank space: %q", got)
	}
}

func TestRegionClampsWideRows(t *testing.T) {
	// Rows arrive from tool data, so their width is not under the region's
	// control. A row that overruns must be cut, not wrapped: wrapping would
	// silently add a line and shear the columns beside it.
	r := NewRegion("R", 20)
	r.Add(strings.Repeat("x", 200))
	for _, line := range strings.Split(strings.TrimRight(r.Rows[0], "\n"), "\n") {
		if lipgloss.Width(line) > 20 {
			t.Fatalf("row is %d wide, over the region's 20", lipgloss.Width(line))
		}
	}
}

func TestRegionTooNarrowRendersNothing(t *testing.T) {
	// Below the minimum a region is absent rather than unreadable.
	r := NewRegion("R", 4)
	r.Add("something")
	if got := r.Render(newTheme(false, nil)); got != "" {
		t.Errorf("a 4-wide region rendered %q", got)
	}
}

func TestRegionBoxedPutsTheRuleOnTheInnerEdge(t *testing.T) {
	// The rule marks the boundary with the transcript, so its side is decided
	// by where the region sits, not by a constant.
	// The rule only exists on a region wide enough to be one.
	r := NewRegion("R", 20)
	r.Add("a")
	left := stripANSI(r.Boxed(newTheme(false, nil), -1))
	right := stripANSI(r.Boxed(newTheme(false, nil), 1))
	if !strings.HasPrefix(left, "│ ") {
		t.Errorf("a left region put its rule on the wrong edge: %q", left)
	}
	if !strings.HasSuffix(right, " │") {
		t.Errorf("a right region put its rule on the wrong edge: %q", right)
	}
	// Every line carries the rule, or the edge breaks where a row is short.
	for name, out := range map[string]string{"left": left, "right": right} {
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		for i, l := range lines {
			if !strings.Contains(l, "│") {
				t.Errorf("%s region line %d has no rule: %q", name, i, l)
			}
		}
	}
}

func TestRegionBoxedOfNothingIsEmpty(t *testing.T) {
	var nilRegion *Region
	if got := nilRegion.Boxed(newTheme(false, nil), 1); got != "" {
		t.Errorf("a nil region boxed to %q", got)
	}
	if got := NewRegion("R", 2).Boxed(newTheme(false, nil), 1); got != "" {
		t.Errorf("a too-narrow region boxed to %q", got)
	}
}

func TestRowDropsValueBeforeKey(t *testing.T) {
	// At a narrow width the value goes and the identifying key stays. A row
	// that kept the value would show "risk S2" with no way to tell which
	// capability it belonged to.
	r := Row{Marker: "●", Key: "Profile target baseline", Value: "risk S1  authorised target"}
	got := stripANSI(r.String(24))
	if !strings.Contains(got, "Profile") {
		t.Errorf("the key was lost: %q", got)
	}
	if w := lipgloss.Width(got); w > 24 {
		t.Errorf("row is %d wide, want 24", w)
	}
	if !strings.HasPrefix(strings.TrimSpace(got), "●") {
		t.Errorf("the marker was lost: %q", got)
	}
}

func TestRowKeepsBothWhenThereIsRoom(t *testing.T) {
	r := Row{Marker: "●", Key: "analyze", Value: "risk low"}
	got := stripANSI(r.String(40))
	if !strings.Contains(got, "analyze") || !strings.Contains(got, "risk low") {
		t.Errorf("row = %q, want both parts", got)
	}
}

func TestRowAtWidthsTooNarrowForBoth(t *testing.T) {
	// Every width from 1 to 24 must produce a line that fits. This is the range
	// where a row's own arithmetic is most likely to produce something wider
	// than the terminal or a negative repeat count.
	for w := 1; w <= 24; w++ {
		for _, r := range []Row{
			{Marker: "●", Key: "a", Value: "v"},
			{Marker: "●", Key: "a very long capability name", Value: "a very long detail"},
			{Key: strings.Repeat("k", 100), Value: strings.Repeat("v", 100)},
			{Marker: strings.Repeat("m", 50)},
		} {
			got := r.String(w)
			if lipgloss.Width(got) > w {
				t.Fatalf("Row%+v at width %d rendered %d wide", r, w, lipgloss.Width(got))
			}
		}
	}
}

func TestRowWithNoValue(t *testing.T) {
	if got := stripANSI((Row{Key: "plain"}).String(20)); got != "plain" {
		t.Errorf("got %q, want plain", got)
	}
	if got := stripANSI((Row{}).String(20)); got != "" {
		t.Errorf("an empty row rendered %q", got)
	}
}

func TestKeyHintKeepsTheKey(t *testing.T) {
	// The key is the part a person looks up; the description is the part they
	// can afford to lose.
	for _, w := range []int{6, 8, 10, 20, 40} {
		got := stripANSI(keyHint(newTheme(false, nil), "F1", "capabilities", w))
		if !strings.Contains(got, "F1") {
			t.Errorf("width %d: the key was lost: %q", w, got)
		}
		if lipgloss.Width(got) > w {
			t.Errorf("width %d: hint is %d wide", w, lipgloss.Width(got))
		}
	}
	if got := keyHint(newTheme(false, nil), "", "orphan", 20); got != "" {
		t.Errorf("a hint with no key rendered %q", got)
	}
}
