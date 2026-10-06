package tui

import "regexp"

// ansiCode is one terminal escape sequence.
//
// The pattern has to cover every form a wrapper or a tool actually emits, not
// just the colour ones. Colour is what the highlighter adds itself, so a CSI
// colour code is the easy case; the sequences that arrive from elsewhere are the
// ones that break things:
//
//   - OSC 8 hyperlinks and window titles. These carry a payload with a colon and
//     slashes in it, and they are terminated by BEL or ST rather than by a final
//     byte in the CSI range. A pattern that stops at the CSI alternative leaves
//     the payload behind as visible text.
//   - The charset selectors, G0 and G1, that some wrappers wrap their output in.
//     These are two bytes long and are not CSI at all.
//   - CSI sequences carrying private parameter bytes and intermediate bytes,
//     which is what cursor movement and erase look like.
//
// A leftover escape is not cosmetic. Layout is computed on visible width, and
// an unstripped byte is a character that occupies a column, so the wrapper
// reserves space for text that never reaches the screen and the line breaks
// early. Worse, a partial match can leave a bare ESC in the string, and that is
// the exact byte a wrapper would then slice through.
var ansiCode = regexp.MustCompile(`\x1b\[[0-9;:?]*[ -/]*[@-~]|\x1b\][^\x1b\x07]*(?:\x07|\x1b\\)|\x1b[()][A-Z0-9]|\x1b[=>]`)

// stripANSI removes escape sequences from a string.
//
// Layout decisions have to be made on visible width, but the strings being
// measured are already styled, so the styling has to come off first.
//
// This is the single place stripping happens. A second, narrower pattern lived
// here next to a broader one in events.go, and the two disagreed: stripping for
// layout and stripping for display were different operations, so a line could
// measure one width and render another.
func stripANSI(s string) string {
	if len(s) == 0 {
		return s
	}
	// Most tool output carries no escapes at all. Skipping the regexp when there
	// is no ESC keeps the common case off the slow path, which matters because
	// this runs once per line of a long transcript.
	if !containsESC(s) {
		return s
	}
	return ansiCode.ReplaceAllString(s, "")
}

func containsESC(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			return true
		}
	}
	return false
}
