package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// displayWidth is the measurement every wrap assertion is made against: the
// columns the line actually occupies on screen.
func displayWidth(s string) int { return lipgloss.Width(s) }

// withoutSpaces strips every space, for the assertions that care only that the
// content of a wrapped line survives and stays in order.
func withoutSpaces(line string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' {
			return -1
		}
		return r
	}, line)
}

func withoutSpacesAll(lines []string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' {
			return -1
		}
		return r
	}, strings.Join(lines, ""))
}

// A line that already fits is returned exactly as it came in. This is the whole
// contract of the wrapper: wrapping is a fallback for lines that do not fit, and
// a fallback that fires on lines that do fit is not a fallback.
func TestWrapLeavesFittingLinesAlone(t *testing.T) {
	for _, s := range []string{
		"",
		"ok",
		"exactly this wide",
		"  indented, and still short",
		"a  b     c",
		"trailing spaces   ",
	} {
		got := wrapText(s, 80)
		if len(got) != 1 {
			t.Errorf("wrapText(%q) = %d lines, want 1: %q", s, len(got), got)
			continue
		}
		if got[0] != s {
			t.Errorf("wrapText(%q) = %q, want it unchanged", s, got[0])
		}
	}
}

// The test of the whole redesign. Tool output is full of tables, and a table is
// nothing but runs of spaces holding columns apart. A wrapper that trims or
// collapses those runs turns the table into a paragraph, which is the specific
// failure this wrapper was written to prevent.
func TestWrapKeepsTableColumnsAligned(t *testing.T) {
	// Built by padding rather than typed out, because a column one space out of
	// place would fail the test for the wrong reason.
	const nameCol, statusCol = 18, 30
	row := func(name, status, duration string) string {
		out := pad(name, nameCol) + pad(status, statusCol-nameCol) + duration
		return out
	}
	table := []string{
		row("NAME", "STATUS", "DURATION"),
		row("render-block", "ok", "1.2s"),
		row("gradient-scan", "degraded", "45.0s"),
		row("permutation-matrix", "failed", "0.4s"),
	}
	second := []string{"ok", "degraded", "failed"}
	third := []string{"1.2s", "45.0s", "0.4s"}
	for i, line := range table {
		got := wrapText(line, 80)
		if len(got) != 1 {
			t.Fatalf("a table row was wrapped at 80:\n%s", strings.Join(got, "\n"))
		}
		if at := strings.Index(got[0], line[:4]); at != 0 {
			t.Errorf("row %d name column moved to %d:\n%s", i, at, got[0])
		}
		if i == 0 {
			if at := strings.Index(got[0], "STATUS"); at != nameCol {
				t.Errorf("STATUS at offset %d, want %d:\n%s", at, nameCol, got[0])
			}
			if at := strings.Index(got[0], "DURATION"); at != statusCol {
				t.Errorf("DURATION at offset %d, want %d:\n%s", at, statusCol, got[0])
			}
			continue
		}
		// The columns have to start at the same offset in every row, which they
		// only do if the gaps survived. Checking offsets rather than content is
		// the point: the values differ per row, the columns do not.
		if at := strings.Index(got[0], second[i-1]); at != nameCol {
			t.Errorf("row %d second column at offset %d, want %d:\n%s", i, at, nameCol, got[0])
		}
		if at := strings.Index(got[0], third[i-1]); at != statusCol {
			t.Errorf("row %d third column at offset %d, want %d:\n%s", i, at, statusCol, got[0])
		}
	}
}

// The same table, too wide for the terminal. The rows still have to be aligned
// with each other after wrapping, which means the gap before a column has to be
// carried onto the continuation line rather than trimmed at the break.
func TestWrapKeepsColumnsAlignedWhenTheTableIsTooWide(t *testing.T) {
	// "STATUS" starts at offset 24 in every row.
	rows := []string{
		"NAME                            STATUS      DURATION",
		"render-block                    ok          1.2s",
		"gradient-scan                   degraded    45.0s",
	}
	const width = 44
	got := make([][]string, len(rows))
	for i, row := range rows {
		got[i] = wrapText(row, width)
		for _, line := range got[i] {
			if len(line) > width {
				t.Errorf("row %d overflowed: %d > %d:\n%s", i, len(line), width, line)
			}
		}
	}
	// Wrapping is allowed to break rows, but not to lose, reorder or re-space the
	// content. The spaces that separated the columns are stripped here because
	// the wrapper is explicitly free to consume the ones it breaks at.
	for i, row := range rows {
		if stripped, strippedWant := withoutSpacesAll(got[i]), withoutSpaces(row); stripped != strippedWant {
			t.Errorf("row %d changed content:\n got %q\nwant %q", i, stripped, strippedWant)
		}
	}
}

// Indentation is structure in tree output. A wrapper that strips it produces a
// flat list, and a wrapper that re-indents continuation lines produces a tree
// with a level that the tool never emitted.
func TestWrapPreservesIndentationWithoutInventingAny(t *testing.T) {
	in := "  ├─ nested child"
	got := wrapText(in, 80)
	if len(got) != 1 || got[0] != in {
		t.Fatalf("a fitting indented line changed: %q", got)
	}

	// Wide enough to force a break. The first line keeps its own indent; the
	// continuation starts flush, because the caller already indents every line.
	long := "  ├─ a child with a name long enough to need breaking here"
	got = wrapText(long, 30)
	if !strings.HasPrefix(got[0], "  ├─ ") {
		t.Errorf("first line lost its indent: %q", got[0])
	}
	for i, line := range got[1:] {
		if strings.HasPrefix(line, " ") {
			t.Errorf("continuation %d inherited an indent it should not have: %q", i+1, line)
		}
	}
	// And the content survives the break.
	if joined := strings.Join(strings.Fields(strings.Join(got, " ")), " "); joined != strings.Join(strings.Fields(long), " ") {
		t.Errorf("content changed across the break:\n got %q\nwant %q", joined, strings.Join(strings.Fields(long), " "))
	}
}

// Indentation wider than the line is dropped rather than wrapping into a line
// that is nothing but spaces.
func TestWrapDropsIndentationThatCannotFit(t *testing.T) {
	got := wrapText("          word", 8)
	for i, line := range got {
		if strings.TrimSpace(line) == "" {
			t.Errorf("line %d is blank: %q", i, line)
		}
	}
}

// A long word is split, not dropped. Losing a token silently is the worst
// available outcome, because the reader has no way to tell it happened.
func TestWrapHardSplitsAWordLongerThanTheLine(t *testing.T) {
	const word = "abcdefghijklmnopqrstuvwxyz0123456789"
	got := wrapText(word, 10)
	if len(got) < 4 {
		t.Fatalf("a 36-column word at width 10 became %d lines: %q", len(got), got)
	}
	for _, line := range got {
		if len(line) > 10 {
			t.Errorf("split line is %d wide: %q", len(line), line)
		}
	}
	if joined := strings.Join(got, ""); joined != word {
		t.Errorf("the split lost content:\n got %q\nwant %q", joined, word)
	}
}

// A long word is split even when it follows real text, and the split does not
// swallow the word before it.
func TestWrapHardSplitsInsideALongSentence(t *testing.T) {
	got := wrapText("see "+strings.Repeat("z", 40), 12)
	// Every space here is either consumed at a break or is the single one before
	// the token, so the assertion is on the characters: nothing dropped, nothing
	// reordered, nothing doubled up.
	if got, want := withoutSpacesAll(got), "see"+strings.Repeat("z", 40); got != want {
		t.Errorf("content changed:\n got %q\nwant %q", got, want)
	}
	for _, line := range got {
		if w := displayWidth(line); w > 12 {
			t.Errorf("line is %d wide: %q", w, line)
		}
	}
	// The word before the split token is not eaten by it, and the space between
	// them is the break, so it is the one space that does not appear.
	if got[0] != "see" {
		t.Errorf("the leading word did not survive alone on its line: %q", got[0])
	}
}

// Tabs are expanded rather than counted as one column, which is what made a
// tab-indented line overflow the transcript.
func TestWrapExpandsTabsBeforeMeasuring(t *testing.T) {
	got := wrapText("\tone\ttwo\tthree", 12)
	for _, line := range got {
		if strings.ContainsRune(line, '\t') {
			t.Errorf("a tab survived into the transcript: %q", line)
		}
		if len(line) > 12 {
			t.Errorf("expanded line is %d wide: %q", len(line), line)
		}
	}
	if joined := strings.Join(strings.Fields(strings.Join(got, " ")), " "); joined != "one two three" {
		t.Errorf("content changed: %q", joined)
	}
}

// Width is display width. A line of full-width characters is wider than its rune
// count, and measuring runes lets it run past the right edge of the terminal.
func TestWrapMeasuresDisplayWidthNotRunes(t *testing.T) {
	got := wrapText(strings.Repeat("測試文字", 8), 10)
	for _, line := range got {
		if w := displayWidth(line); w > 10 {
			t.Errorf("line is %d display columns, over the 10 budget: %q", w, line)
		}
	}
	if joined := strings.Join(got, ""); joined != strings.Repeat("測試文字", 8) {
		t.Errorf("wide characters were lost or clipped")
	}
}

// CR is a carriage return, not content; a line that ends in one is not one
// column wider than it looks.
func TestWrapTrimsATrailingCarriageReturn(t *testing.T) {
	got := wrapText("ok\r", 40)
	if len(got) != 1 || got[0] != "ok" {
		t.Errorf("wrapText(%q) = %q, want [ok]", "ok\r", got)
	}
}

// A width too small to wrap in is not a reason to lose the line.
func TestWrapKeepsTheLineAtAbsurdWidths(t *testing.T) {
	in := "a line that cannot be wrapped anywhere useful"
	for _, width := range []int{0, -1, 1, 7} {
		got := wrapText(in, width)
		if len(got) != 1 || got[0] != in {
			t.Errorf("width %d: got %q, want the line back untouched", width, got)
		}
	}
}

// Wrapping is a fallback, so ordinary prose must still break where a reader
// expects it to: at a space, never mid-word, and as few lines as the width
// allows.
func TestWrapBreaksProseAtSpaces(t *testing.T) {
	const width = 40
	s := "the quick brown fox jumps over the lazy dog and keeps going for a while"
	got := wrapText(s, width)
	want := []string{
		"the quick brown fox jumps over the lazy",
		"dog and keeps going for a while",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d:\n%q", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d:\n got %q\nwant %q", i, got[i], want[i])
		}
	}
}

// A break is only ever taken at a space, so no prose line ends mid-word. This is
// the property the space-preserving wrapper could plausibly have lost.
func TestWrapNeverBreaksProseMidWord(t *testing.T) {
	const width = 40
	for _, s := range []string{
		"rendering the block took longer than expected because the gradient scan " +
			"has to walk every pixel and compare it against the previous frame",
		"permutation matrix row 4 of 9: 812,447 cells compared, 3 mismatches retained",
		strings.Repeat("word ", 30),
	} {
		got := wrapText(s, width)
		for i, line := range got {
			if w := displayWidth(line); w > width {
				t.Errorf("line %d is %d wide: %q", i, w, line)
			}
			if line != strings.TrimSpace(line) {
				t.Errorf("line %d has untrimmed edges: %q", i, line)
			}
		}
		// Nothing was dropped or reordered.
		if joined := strings.Join(strings.Fields(strings.Join(got, " ")), " "); joined != strings.Join(strings.Fields(s), " ") {
			t.Errorf("content changed:\n got %q\nwant %q", joined, strings.Join(strings.Fields(s), " "))
		}
	}
}

// Every break line is filled as far as it goes. Leaving a word short of the edge
// when the next one would not fit is correct; leaving the line half empty is not.
func TestWrapFillsEachLineBeforeBreaking(t *testing.T) {
	const width = 24
	got := wrapText("aa bb cc dd ee ff gg hh ii jj kk ll", width)
	for i, line := range got {
		if i < len(got)-1 && displayWidth(line) < width {
			// The next word would have had to fit in the remainder.
			next := strings.Fields(got[i+1])[0]
			if displayWidth(line)+1+displayWidth(next) <= width {
				t.Errorf("line %d broke early at %d of %d columns: %q", i, displayWidth(line), width, line)
			}
		}
	}
}
