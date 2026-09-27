package tui

import (
	"os"

	"github.com/mattn/go-isatty"
)

// isTerminal reports whether f is an interactive terminal.
//
// The test is a real isatty rather than a character-device check, and the
// difference is not academic. /dev/null, /dev/zero and every other character
// device satisfy os.ModeCharDevice without being a terminal, so a
// character-device test reports a session should start when it must not: the
// interface then draws into a stream nobody is watching, still consumes the
// operator's keystrokes, and reports a failure for a command that ran
// perfectly well. `tool tui >/dev/null 2>/dev/null` behaved differently from
// `tool tui >file` for exactly this reason.
//
// isatty is also what agrees with the terminal emulator on the other end, which
// is the only definition that matters when the question is whether a person is
// present.
//
// A false result is the safe direction: the caller falls back to normal CLI
// behaviour.
func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	// A real isatty, not a character-device test. /dev/null, /dev/zero and
	// every other character device satisfy ModeCharDevice without being a
	// terminal, and a session launched into one of them draws nowhere while
	// still consuming input and reporting failures. isatty is also the only
	// check that agrees with the terminal emulator on the other end.
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

// IsInteractive reports whether an interactive session should be started on
// the given output stream.
//
// Tools call this before launching the TUI so that a piped or redirected
// invocation -- `tool scan target | tee out`, CI, a cron job -- falls through
// to the ordinary one-shot CLI instead of filling a pipe with escape codes.
func IsInteractive(out *os.File) bool {
	if out == nil {
		return false
	}
	// Both ends matter: a terminal on stdout with a redirected stderr is
	// still unusable, because the TUI owns both.
	return isTerminal(out) && isTerminal(os.Stderr)
}
