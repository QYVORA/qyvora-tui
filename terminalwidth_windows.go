//go:build windows

package tui

// TerminalWidth reports the column count of the terminal attached to stdout.
// Windows builds have no portable ioctl path here, so the width is reported as
// unconstrained and callers render the fullest banner form.
func TerminalWidth() int {
	return 0
}
