package tui

import "os"

// isTerminal reports whether f is an interactive terminal.
//
// It is a deliberate, minimal check rather than a dependency: the TUI needs to
// answer exactly one question, "is there a human at the other end", and
// answering it must not pull a terminal-detection package into every tool's
// dependency graph. A false result is the safe direction -- the caller falls
// back to normal CLI behaviour.
func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
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
