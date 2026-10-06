package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// stripANSI is the single stripping path now, and layout is measured with it, so
// the two operations that used to disagree have to agree. These are the sequences
// that made the narrow pattern wrong.
func TestStripANSIRemovesEverySequenceForm(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"colour", "\x1b[31mred\x1b[0m", "red"},
		{"truecolor", "\x1b[38;2;255;110;133mred\x1b[0m", "red"},
		{"cursor movement", "a\x1b[2Kb", "ab"},
		{"erase", "a\x1b[2Kb\x1b[J", "ab"},
		{"private parameters", "\x1b[?25lhidden\x1b[?25h", "hidden"},
		{"intermediate bytes", "\x1b[1$q ok", " ok"},
		{"OSC 8 hyperlink with BEL", "see \x1b]8;;https://example.com\x07here\x1b]8;;\x07", "see here"},
		{"OSC 8 hyperlink with ST", "see \x1b]8;;https://example.com\x1b\\here\x1b]8;;\x1b\\", "see here"},
		{"OSC window title", "\x1b]0;a title with spaces\x07body", "body"},
		{"charset selector", "\x1b(Bbody\x1b(B", "body"},
		{"keypad mode", "\x1b=body\x1b>", "body"},
		{"several in one line", "\x1b[1m\x1b[31ma\x1b[0m\x1b[32mb\x1b[0m", "ab"},
	} {
		if got := stripANSI(tc.in); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

// No escape, no allocation and the same string back. This is the common case
// for a tool transcript, and it should not pay for the regexp.
func TestStripANSILeavesPlainTextAlone(t *testing.T) {
	for _, s := range []string{"", "plain", "  indented", "tab\there", "line with  spaces"} {
		if got := stripANSI(s); got != s {
			t.Errorf("got %q, want %q", got, s)
		}
	}
}

// A leftover ESC is the byte that gets sliced through by a wrapper, so a string
// that had escapes in it must have none left. Checking for the byte directly is
// stricter than re-running the pattern.
func TestStripANSILeavesNoEscapeByte(t *testing.T) {
	for _, s := range []string{
		"\x1b[31mred\x1b[0m",
		"\x1b]8;;https://example.com\x07link\x1b]8;;\x07",
		"\x1b(Btext\x1b(B",
		"a\x1b[2Kb\x1b[J",
		"\x1b[?25lhidden\x1b[?25h",
	} {
		if containsESC(stripANSI(s)) {
			t.Errorf("%q still contains an escape", stripANSI(s))
		}
	}
}

// Stripping for layout and stripping for display were two different operations.
// If they ever diverge again, a line measures one width and renders another, and
// the only symptom is a box that is subtly the wrong size.
func TestStripANSIAndVisibleLineAgreeOnStripping(t *testing.T) {
	for _, s := range []string{
		"\x1b[31mred\x1b[0m",
		"\x1b]8;;https://example.com\x07link\x1b]8;;\x07",
		"\x1b(Btext\x1b(B",
		"a\x1b[2Kb\x1b[J",
		"plain text",
	} {
		want := stripANSI(s)
		if got := visibleLine(s); got != want {
			t.Errorf("%q: visibleLine gave %q, stripANSI gave %q", s, got, want)
		}
	}
}

// The carriage-return collapse happens first, so a redrawn progress line must
// still end up with no escapes after both steps.
func TestVisibleLineCollapsesThenStrips(t *testing.T) {
	in := "\x1b[2K\r\x1b[1;33mProbing 10%\x1b[0m\rProbing 100%"
	if got, want := visibleLine(in), "Probing 100%"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Stripping is what makes the transcript safe to measure, so the width the
// wrapper reserves must be the width the reader sees. The escaped and unescaped
// forms of the same line have to measure identically.
func TestStrippedWidthMatchesTheVisibleText(t *testing.T) {
	for _, tc := range []struct{ styled, plain string }{
		{"\x1b[31mred\x1b[0m", "red"},
		{"\x1b[1mNAME\x1b[0m      STATUS", "NAME      STATUS"},
		{"\x1b[2K\x1b[31mrunning\x1b[0m", "running"},
	} {
		styled, plain := tc.styled, tc.plain
		if got := lipgloss.Width(stripANSI(styled)); got != lipgloss.Width(plain) {
			t.Errorf("%q: stripped width %d, want %d", styled, got, lipgloss.Width(plain))
		}
	}
}
