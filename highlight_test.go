package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// coloredTheme is a truecolor theme, so the tests below exercise the real
// rendering path rather than the identity transform that plainTheme returns.
func coloredTheme(t *testing.T) Theme {
	t.Helper()
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	lipgloss.SetColorProfile(termenv.TrueColor)
	return newTheme(true, nil)
}

// The round trip is the property that makes highlighting safe to switch on: take
// the styling off a highlighted line and the tool's own bytes are back, exactly.
// A highlighter that dropped a character, reflowed a space or changed a
// capital would be quietly editing the tool's output while claiming only to
// colour it.
func TestHighlightRoundTripsToTheToolsOwnBytes(t *testing.T) {
	th := coloredTheme(t)
	for _, s := range []string{
		"",
		"plain output with no recognisable token at all",
		"CRITICAL: unsafe pointer dereference in src/main.c:412",
		"warning: unused variable 'count' (severity low)",
		"scan ok in 1.2s across 812,447 cells",
		"GET https://example.com/api/v1/users?page=2 -> 200",
		"  NAME              STATUS      DURATION",
		"render-block      ok          1.2s",
		"gradient-scan     degraded    45.0s",
		"├─ child one",
		"└─ child two",
		"mixed ERROR and warning and note and failed and cancelled",
		"::error file=src/a.rs,line=9: cannot find value `x`",
		"0 warnings, 0 errors — done",
	} {
		got := stripANSI(highlightLine(s, th))
		if got != s {
			t.Errorf("round trip changed the text:\n got %q\nwant %q", got, s)
		}
	}
}

// Highlighting must not change what a line looks like on screen. Every escape
// sequence is zero-width, so if the visible width moved then the highlighter is
// painting over something it did not intend to.
func TestHighlightDoesNotChangeVisibleWidth(t *testing.T) {
	th := coloredTheme(t)
	for _, s := range []string{
		"CRITICAL: unsafe pointer in src/main.c:412",
		"scan ok in 1.2s across 812,447 cells",
		"GET https://example.com/api/v1/users?page=2 -> 200",
		"NAME              STATUS      DURATION",
		"",
	} {
		if got, want := lipgloss.Width(highlightLine(s, th)), lipgloss.Width(s); got != want {
			t.Errorf("width %d, want %d, for %q", got, want, s)
		}
	}
}

// The ordering constraint, stated as a test: wrapping a line and then
// highlighting it must leave no escape sequence split in half.
//
// A wrapper that measures a styled string counts the escapes as characters, so
// it breaks mid-sequence and the rest of the line inherits a colour that was
// never closed. The assertion is that every escape in the result is well formed.
func TestHighlightRunsAfterWrappingSoNoSequenceIsSplit(t *testing.T) {
	th := coloredTheme(t)
	for _, width := range []int{20, 32, 48, 60} {
		for _, raw := range []string{
			"CRITICAL: unsafe pointer dereference in src/main.c:412 while scanning the permutation matrix",
			"the scan reported ok for every one of the 812,447 cells it compared against the baseline",
			"warning: the gradient scan degraded and was skipped for https://example.com/very/long/path",
		} {
			for _, line := range wrapText(raw, width) {
				styled := highlightLine(line, th)
				if !sequencesWellFormed(styled) {
					t.Errorf("width %d: a split escape sequence in %q", width, styled)
				}
				if got := stripANSI(styled); got != line {
					t.Errorf("width %d: highlighting after wrapping changed the text:\n got %q\nwant %q", width, got, line)
				}
			}
		}
	}
}

// sequencesWellFormed reports whether every ESC in s begins a sequence the
// ansiCode pattern matches in full. A truncated sequence leaves an ESC that no
// match consumes.
func sequencesWellFormed(s string) bool {
	rest := s
	for {
		i := strings.IndexByte(rest, 0x1b)
		if i < 0 {
			return true
		}
		tail := rest[i:]
		loc := ansiCode.FindStringIndex(tail)
		if loc == nil || loc[0] != 0 {
			return false
		}
		rest = tail[loc[1]:]
	}
}

// A line the highlighter recognises is actually styled. Without this the other
// tests pass just as well when the highlighter does nothing at all.
func TestHighlightActuallyStylesTheTokensItClaims(t *testing.T) {
	th := coloredTheme(t)
	for _, tc := range []struct{ in, want string }{
		{"CRITICAL: something broke", "CRITICAL"},
		{"severity: warning", "warning"},
		{"scan ok", "ok"},
		{"the run failed", "failed"},
		{"took 45.0s", "45.0s"},
		{"see src/main.c", "src/main.c"},
		{"fetch https://example.com/x", "https://example.com/x"},
	} {
		got := highlightLine(tc.in, th)
		if got == th.Output.Render(tc.in) {
			t.Errorf("%q was not styled at all", tc.in)
			continue
		}
		if !strings.Contains(stripANSI(got), tc.want) {
			t.Errorf("%q: %q is not in the output", tc.in, tc.want)
		}
	}
}

// With colour off, highlighting is a no-op. The plain-text path must not depend
// on a terminal profile being present at all, which is the case that matters for
// pipes and for CI.
func TestHighlightIsANoOpWithoutColour(t *testing.T) {
	th := newTheme(false, nil)
	for _, s := range []string{"CRITICAL: broke", "scan ok in 1.2s", "src/main.c:412"} {
		if got := highlightLine(s, th); got != s {
			t.Errorf("no-colour highlight of %q returned %q", s, got)
		}
	}
}

// A line that already carries an escape sequence is passed straight through. A
// runner can inject colour codes, and re-styling around a sequence the
// highlighter does not understand is how that sequence ends up half-consumed.
func TestHighlightLeavesForeignEscapeSequencesAlone(t *testing.T) {
	th := coloredTheme(t)
	in := "\x1b[1;31mCRITICAL\x1b[0m: broken in src/main.c"
	got := highlightLine(in, th)
	if got != th.Output.Render(in) {
		t.Errorf("a line with foreign escapes was restyled:\n got %q\nwant %q", got, th.Output.Render(in))
	}
}

// The table is the case the whole redesign turned on. Alignment is carried by
// runs of spaces, so a highlighter that reflowed or trimmed them would undo the
// wrapper's work and the transcript would go back to being a paragraph.
func TestHighlightPreservesTableAlignment(t *testing.T) {
	th := coloredTheme(t)
	rows := []string{
		"NAME              STATUS      DURATION",
		"render-block      ok          1.2s",
		"gradient-scan     degraded    45.0s",
	}
	for i, row := range rows {
		got := stripANSI(highlightLine(row, th))
		if got != row {
			t.Errorf("row %d changed:\n got %q\nwant %q", i, got, row)
			continue
		}
		// The second column has to start at the same offset in every row.
		if at := strings.Index(got, "STATUS"); i == 0 && at != 18 {
			t.Errorf("header STATUS at %d, want 18", at)
		}
		for _, v := range []string{"ok", "degraded"} {
			if strings.Contains(got, v) {
				if at := strings.Index(got, v); at != 18 {
					t.Errorf("row %d: %q at %d, want 18", i, v, at)
				}
			}
		}
	}
}

// Tree indentation is structure too, and the same argument applies to it.
func TestHighlightPreservesTreeDrawing(t *testing.T) {
	th := coloredTheme(t)
	for _, row := range []string{
		"├─ child one",
		"│  └─ grandchild ok",
		"└─ child two",
		"   ╰─ leaf failed",
	} {
		if got := stripANSI(highlightLine(row, th)); got != row {
			t.Errorf("tree row changed:\n got %q\nwant %q", got, row)
		}
	}
}

// A token is styled once. Painting a span twice means two classes both decided
// the same characters meant something, which is the ambiguity that produces an
// unreadable pile of conflicting escapes.
//
// The claim is made on the segmentation rather than on the rendered string, so
// the test is not inferring intent by reading escape sequences back out.
func TestHighlightStylesEachSpanOnce(t *testing.T) {
	th := coloredTheme(t)
	for _, s := range []string{
		"error in src/error.c",
		"warning in /var/log/warning.log",
		"ok https://ok.example.com/ok",
		"CRITICAL error high medium low info",
	} {
		segs := highlightSegments(s, th)
		// Segments must tile the line exactly: every byte in it, in order, with
		// no overlap and nothing dropped. An overlap is the double-styling.
		if joined := joinSegments(segs); joined != s {
			t.Errorf("%q: segments do not reconstruct the line:\n got %q\nwant %q", s, joined, s)
		}
		for i := 1; i < len(segs); i++ {
			if sameStyle(segs[i].style, segs[i-1].style) {
				// Two adjacent runs in the same style should have been one run.
				// Not a failure on its own, but it means the split was pointless
				// and every one of them costs two escape sequences.
				t.Errorf("%q: adjacent segments %d and %d share a style: %q then %q",
					s, i-1, i, segs[i-1].text, segs[i].text)
			}
		}
		// Each Render costs one set and one reset, so the sequence count follows
		// from the segment count and nothing else.
		want := 0
		for _, seg := range segs {
			if seg.text != "" {
				want += 2
			}
		}
		if got := countSeq(highlightLine(s, th)); got != want {
			t.Errorf("%q: %d sequences, want %d for %d segments", s, got, want, len(segs))
		}
	}
}

// The severity word inside a path is the overlap that matters in practice: a
// file called error.c is a file, and painting its name as a failure is a false
// alarm about the run.
func TestHighlightDoesNotPaintSeverityWordsInsidePaths(t *testing.T) {
	th := coloredTheme(t)
	for _, s := range []string{
		"error in src/error.c",
		"failed: /var/log/failed.log is unreadable",
		"warning at /etc/warning.d/99-x.conf",
	} {
		// A path must arrive whole, extension included. The trailing component is
		// the part a pattern ending on a bare word boundary leaves behind, and
		// "error" in the middle of it must not be claimed as a severity on the
		// way past.
		var paths []string
		for _, seg := range highlightSegments(s, th) {
			// Only styled segments are candidates. An unstyled gap that happens to
			// contain a slash is the space around the path, not part of it.
			if !strings.Contains(seg.text, "/") || sameStyle(seg.style, th.Output) {
				continue
			}
			paths = append(paths, seg.text)
		}
		want := map[string]string{
			"error in src/error.c":                      "src/error.c",
			"failed: /var/log/failed.log is unreadable": "/var/log/failed.log",
			"warning at /etc/warning.d/99-x.conf":       "/etc/warning.d/99-x.conf",
		}[s]
		if len(paths) != 1 || paths[0] != want {
			t.Errorf("%q: paths %q, want exactly [%q]", s, paths, want)
		}
	}
}

// sameStyle reports whether two styles draw text the same way. The style type is
// a struct wrapping a closure, so identity comparison is not available and the
// only honest question is whether the rendering matches.
func sameStyle(a, b lipglossStyle) bool {
	return a.Render("x") == b.Render("x")
}

func joinSegments(segs []segment) string {
	var b strings.Builder
	for _, seg := range segs {
		b.WriteString(seg.text)
	}
	return b.String()
}

func isSeverityWord(s string) bool {
	for _, w := range []string{"critical", "fatal", "error", "err", "high",
		"medium", "moderate", "warn", "warning", "low", "note",
		"info", "debug", "trace"} {
		if s == w {
			return true
		}
	}
	return false
}

// countSeq counts the escape sequences in a string.
func countSeq(s string) int {
	n := 0
	for {
		loc := ansiCode.FindStringIndex(s)
		if loc == nil {
			return n
		}
		n++
		s = s[loc[1]:]
	}
}

// Nothing matches, so nothing is styled and the line is the Output style whole.
func TestHighlightLeavesUnrecognisedTextAlone(t *testing.T) {
	th := coloredTheme(t)
	for _, s := range []string{
		"a sentence with no tokens in it whatsoever",
		"  ",
		"---- separators and dots....",
	} {
		if got := highlightLine(s, th); got != th.Output.Render(s) {
			t.Errorf("%q was styled but nothing in it matches:\n got %q\nwant %q", s, got, th.Output.Render(s))
		}
	}
}
