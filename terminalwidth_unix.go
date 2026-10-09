//go:build !windows

package tui

import (
	"os"

	"github.com/mattn/go-isatty"
	"golang.org/x/sys/unix"
)

// TerminalWidth reports the column count of the terminal attached to stdout,
// or 0 when the width cannot be measured: stdout is not a terminal, or the
// size is unavailable. Zero means "unconstrained", so a caller renders the
// fullest banner form and lets the output medium decide.
func TerminalWidth() int {
	if os.Stdout == nil || !isatty.IsTerminal(os.Stdout.Fd()) {
		return 0
	}
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil {
		return 0
	}
	return int(ws.Col)
}
