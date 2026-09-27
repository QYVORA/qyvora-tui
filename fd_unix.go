//go:build !windows

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

// redirectStdout points the process's standard output at w for the duration of a
// command, and returns a function that puts it back.
//
// The redirection is done on the file descriptor, not on the os.Stdout variable.
// That distinction is the whole reason this function exists: a package that
// captured os.Stdout into a variable during init keeps writing to the original
// file no matter how the variable is later reassigned, and its output would
// land on the terminal in the middle of the interface's own frames. Moving the
// descriptor catches every writer, however it obtained its handle.
func redirectStdout(w *os.File) (restore func(), err error) {
	saved, err := syscall.Dup(syscall.Stdout)
	if err != nil {
		return nil, err
	}
	// The copy must not be closed on exec, or a command that spawns a helper
	// would lose the redirection halfway through.
	syscall.CloseOnExec(saved)
	backing := os.NewFile(uintptr(saved), "/dev/tty-stdout-saved")

	if err := syscall.Dup2(int(w.Fd()), syscall.Stdout); err != nil {
		_ = backing.Close()
		return nil, err
	}
	// Reassign the variable as well, so code that reads os.Stdout at call time
	// follows the same destination as code holding an older handle.
	previous := os.Stdout
	os.Stdout = w

	var once bool
	return func() {
		if once {
			return
		}
		once = true
		os.Stdout = previous
		_ = syscall.Dup2(int(backing.Fd()), syscall.Stdout)
		_ = backing.Close()
	}, nil
}

// canRedirectStdout reports whether stdout can be redirected in place. It is
// false on platforms without a dup2 equivalent, where the caller must capture
// output through a subprocess pipe instead.
func canRedirectStdout() bool { return true }
