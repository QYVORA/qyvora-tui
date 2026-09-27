//go:build windows

package tui

import "os"

// detachTerminal returns a private duplicate of the process's standard output.
//
// On Windows the console handle cannot be duplicated the way a POSIX file
// descriptor can, so the interface keeps the existing *os.File. That is safe
// because the in-process capture path on this platform is limited anyway -- see
// redirectStdout.
func detachTerminal() (*os.File, func(), error) {
	return os.Stdout, func() {}, nil
}

// redirectStdout points the process's standard output at w for the duration of
// a command, and returns a function that puts it back.
//
// Windows has no dup2 equivalent for a console handle, so only the variable is
// reassigned. A tool that captured os.Stdout during package initialisation keeps
// writing to the console and will corrupt the display; RedirectsUnsupported
// reports that, and the runner falls back to spawning the tool as a child
// process, where the capture pipe is the child's real standard output.
func redirectStdout(w *os.File) (restore func(), err error) {
	previous := os.Stdout
	os.Stdout = w
	var once bool
	return func() {
		if once {
			return
		}
		once = true
		os.Stdout = previous
	}, nil
}

// redirectStderr points the process's standard error at w for the duration of
// a command, and returns a function that puts it back.
func redirectStderr(w *os.File) (restore func(), err error) {
	previous := os.Stderr
	os.Stderr = w
	var once bool
	return func() {
		if once {
			return
		}
		once = true
		os.Stderr = previous
	}, nil
}

// canRedirectStdout reports whether the output streams can be redirected at the
// descriptor level. On Windows they cannot, so the caller must capture through
// a subprocess.
func canRedirectStdout() bool { return false }
