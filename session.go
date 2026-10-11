package tui

import (
	"os"
	"strings"
)

// TUISessionEnv is the environment variable the TUI's runners set while they
// are executing a command on the interactive console's behalf.
//
// It exists to break exactly one ambiguity. A framework prints its own identity
// banner when a command runs -- tool name, tagline, target, timestamp -- and the
// TUI draws a header band carrying the same identity. Under the console both
// would appear, stacked, and the transcript would repeat what the header already
// says. A framework checks this signal before printing and stays quiet when it
// is set.
//
// The signal is deliberately an environment variable set by the runner, not an
// inference from the environment. "Is stdout a TTY" cannot tell a run inside the
// console from a standalone run that has been piped into a log, and both of
// those deserve different treatment; a standalone run must keep its banner
// whether it is piped or not. With the variable absent -- every standalone run,
// and any tool that forgets to consult it -- the default is to print, so the
// failure mode is the current, correct standalone behaviour rather than a
// silently missing banner.
const TUISessionEnv = "QYVORA_TUI"

// InTUISession reports whether the current process is executing a command on
// behalf of the interactive console.
//
// A framework calls it before printing its own banner:
//
//	if !tui.InTUISession() {
//		printBanner()
//	}
//
// It reads the environment on every call rather than caching the answer. The
// in-process runner sets the signal for the duration of one command and clears
// it afterwards, and the same process may print a standalone banner outside a
// run, so a cached value would be wrong the moment the context changed.
func InTUISession() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(TUISessionEnv))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// enterTUISession sets the session signal for the duration of one command and
// returns a function that restores the previous value exactly -- including the
// case where it was not set at all.
//
// It is unexported because only the runner sets the signal, and it is only
// called from inside a capture, which the package already serialises, so the
// process-wide environment is never mutated by two runs at once.
func enterTUISession() func() {
	prev, had := os.LookupEnv(TUISessionEnv)
	_ = os.Setenv(TUISessionEnv, "1")
	return func() {
		if had {
			_ = os.Setenv(TUISessionEnv, prev)
			return
		}
		_ = os.Unsetenv(TUISessionEnv)
	}
}

// childEnv is the environment a TUI-spawned child inherits: the current
// environment plus the session signal.
//
// A child process cannot see the parent's in-process Setenv, so the signal has
// to be placed in its environment explicitly. It is a function of its own so the
// signal can be asserted without spawning a process. The base slice is copied
// rather than appended to in place: os.Environ returns a fresh slice today, but
// appending to a caller's slice is the kind of thing that breaks quietly when
// the caller changes.
func childEnv(base []string) []string {
	env := make([]string, 0, len(base)+1)
	env = append(env, base...)
	return append(env, TUISessionEnv+"=1")
}
