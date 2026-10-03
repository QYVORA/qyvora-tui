package tui

import "time"

// This file holds the parts of the cancellation contract that are the same on
// every platform. They used to live in child_unix.go, which made them
// unavailable to the Windows child implementation: the Unix file had to be
// excluded there to stop its symbols colliding with child_windows.go, and
// excluding it took ExitCancelled with it.

// interruptGrace is how long a cancelled execution is given to shut down
// cleanly after SIGINT before it is killed outright.
//
// A tool that is persisting artifacts, flushing a report or closing a
// database needs a moment to finish doing so. Killing immediately would
// discard exactly the partial results cancellation is supposed to preserve.
const interruptGrace = 3 * time.Second

// ExitCancelled is the conventional shell status for a process interrupted by
// SIGINT. The TUI reports it for any execution whose context was cancelled, so
// a script and the interactive transcript agree on what an interrupt meant.
const ExitCancelled = 130
