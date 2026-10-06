package tui

import "strings"

// The interface is drawn on its own ground rather than on the terminal's.
//
// That sounds like decoration and it is not. Every fill in the palette is a step
// on one ladder, and the steps only mean anything if they are used in order: the
// darkest is the field behind everything, and each lighter step is a surface
// raised off it. An interface that paints its chrome but leaves the transcript on
// whatever background the terminal happens to have has no ladder at all, because
// there is nothing for Surface to be raised above.
//
// Two things make the fill hard, and both are properties of how terminals work.

// ground paints a line onto a ground and keeps the ground under the text.
//
// Lipgloss will happily render a style onto a string that already contains
// styled spans, and the result looks right at a glance. It is not. Every styled
// span in the middle of the line ends with a full reset, and a full reset clears
// the background as well as the foreground, so the ground stops there. What comes
// after the span renders on the terminal's default background, which means a row
// drawn with a ground has a stripe of a different colour running through it
// wherever the interface emphasised anything.
//
// Confirmed by rendering one: an outer fill plus an inner fill produces
//
//	\x1b[48;2;15;29;22mhello\x1b[38;2;255;255;255;48;2;24;44;34mPILL\x1b[0mworld      \x1b[0m
//
// The trailing "world" and its padding carry no background at all. So the ground
// has to be re-established after each reset rather than trusted to wrap the line.
func ground(style lipglossStyle, line string, width int) string {
	if !styleIsActive(style) {
		return line
	}
	line = padTo(clampLine(line, width), width)
	// The set sequence the style emits on its own, captured once per call rather
	// than re-derived by parsing its output.
	set := leadingSGR(style.Render("x"))
	if set == "" {
		return line
	}

	out := make([]byte, 0, len(line)+16)
	// The ground goes down before anything else, so the leading padding of the
	// row is painted as well.
	out = append(out, set...)
	for i := 0; i < len(line); {
		if line[i] != 0x1b {
			out = append(out, line[i])
			i++
			continue
		}
		end := escapeEnd(line, i)
		if end < 0 {
			// An unterminated escape is passed through untouched. It is already
			// broken and guessing where it ends would make it worse.
			out = append(out, line[i:]...)
			break
		}
		seq := line[i:end]
		out = append(out, seq...)
		if isResetSequence(seq) {
			out = append(out, set...)
		}
		i = end
	}
	return string(out)
}

// leadingSGR returns the SGR sequence a rendered style opens with.
func leadingSGR(rendered string) string {
	if len(rendered) == 0 || rendered[0] != 0x1b {
		return ""
	}
	if end := strings.IndexByte(rendered, 'm'); end > 0 {
		return rendered[:end+1]
	}
	return ""
}

// escapeEnd returns the index just past the escape sequence starting at i, or
// -1 when the sequence is unterminated.
//
// The scan has to follow ECMA-48's structure rather than stop at the first byte
// that could be a final one. A CSI is ESC '[' then parameter bytes in 0x30-0x3F,
// intermediate bytes in 0x20-0x2F, then a single final byte in 0x40-0x7E. The
// parameter range overlaps the naive "0x40 and up ends it" test on the digits of
// a reset: '0' is 0x30, and treating it as a final byte cuts "\x1b[0m" in half
// after the bracket, so a reset is never recognised as a reset and the ground is
// never restored after one.
func escapeEnd(s string, i int) int {
	if i+1 >= len(s) {
		return -1
	}
	switch s[i+1] {
	case '[': // control sequence
		j := i + 2
		for j < len(s) && s[j] >= 0x20 && s[j] <= 0x3f {
			j++ // parameters and intermediates
		}
		if j >= len(s) || s[j] < 0x40 || s[j] > 0x7e {
			return -1
		}
		return j + 1
	case ']': // operating system command, terminated by BEL or ST
		for j := i + 2; j < len(s); j++ {
			switch s[j] {
			case 0x07:
				return j + 1
			case 0x1b:
				if j+1 < len(s) && s[j+1] == '\\' {
					return j + 2
				}
				return -1
			}
		}
		return -1
	default:
		// A two-byte escape such as a charset selector or a mode change.
		return i + 2
	}
}

// isResetSequence reports whether an escape sequence is a reset, which is the
// one kind of sequence that clears a background the ground had just established.
func isResetSequence(seq string) bool {
	switch seq {
	case "\x1b[0m", "\x1b[m", "\x1b[00m":
		return true
	}
	return false
}

// styleIsActive reports whether a style will paint anything.
//
// With NO_COLOR, or when a slot resolves to nothing, every style in the theme is
// the identity transform and running the ground machinery over each row would
// cost a full scan of the frame to produce the frame unchanged.
func styleIsActive(style lipglossStyle) bool {
	return style.Render("x") != "x"
}

// groundLines paints a block of rows onto a ground at a width.
//
// The rows are padded first and painted second so that a short row, which is the
// normal case for the tail of a transcript, is filled to the edge instead of
// leaving a stripe of terminal background at the right of every one of them.
func groundLines(style lipglossStyle, lines []string, width int) []string {
	if !styleIsActive(style) {
		return lines
	}
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, ground(style, l, width))
	}
	return out
}
