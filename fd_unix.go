//go:build unix

package tui

import (
	"os"
	"syscall"
)

// detachTerminal returns a private duplicate of the process's standard output
// and restores stdout to its original target.
//
// The interface needs a handle of its own that the TUI can render to for the
// whole session. When a command runs, the process's real standard output is
// pointed at a capture pipe so the tool's output can be read; without a private
// duplicate the renderer would follow stdout into that pipe and the interface
// would overwrite its own transcript.
//
// This is a dup of the file descriptor rather than a copy of the *os.File
// value, because the point is to hold the same underlying terminal while
// stdout itself is moved elsewhere.
func detachTerminal() (*os.File, func(), error) {
	saved, err := syscall.Dup(syscall.Stdout)
	if err != nil {
		// Without a private handle there is no safe way to redirect stdout, so
		// report it and let the caller fall back to a subprocess, whose output
		// is captured by the pipe the parent creates for it.
		return nil, nil, err
	}
	// Clear the close-on-exec flag: this duplicate must survive exec, because
	// it is the handle the interface keeps drawing to.
	syscall.CloseOnExec(saved)
	real := os.NewFile(uintptr(saved), "/dev/tty-render")
	restore := func() {
		if real != nil {
			_ = real.Close()
		}
	}
	return real, restore, nil
}
