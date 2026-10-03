//go:build unix && (aix || darwin || dragonfly || freebsd || netbsd || openbsd || android || (linux && !arm64 && !riscv64 && !loong64))

package tui

import (
	"os"
	"syscall"
)

// These are the unix platforms whose syscall package actually defines Dup2.
// The build tag lists them rather than saying "not windows" because a bare unix
// tag is not enough: linux/arm64, riscv64 and loong64 are unix and do not have
// it either. The linux clause therefore excludes them by name.
//
// A new architecture defaults to the no-dup2 file, which is the right default:
// losing the in-place redirection degrades to a subprocess pipe, whereas
// wrongly claiming to support it corrupts the display.

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

// redirectStderr points the process's standard error at w for the duration of
// a command, and returns a function that puts it back.
//
// This is not optional. A tool prints a substantial part of what the operator
// asked for on standard error: a capability table, a resolved configuration, a
// warning, the explanation of a failure. Leaving stderr attached to the
// terminal while the interface owns the screen means that text lands in the
// middle of the interface's own frames and tears the display apart.
func redirectStderr(w *os.File) (restore func(), err error) {
	saved, err := syscall.Dup(syscall.Stderr)
	if err != nil {
		return nil, err
	}
	syscall.CloseOnExec(saved)
	backing := os.NewFile(uintptr(saved), "/dev/tty-stderr-saved")

	if err := syscall.Dup2(int(w.Fd()), syscall.Stderr); err != nil {
		_ = backing.Close()
		return nil, err
	}
	previous := os.Stderr
	os.Stderr = w

	var once bool
	return func() {
		if once {
			return
		}
		once = true
		os.Stderr = previous
		_ = syscall.Dup2(int(backing.Fd()), syscall.Stderr)
		_ = backing.Close()
	}, nil
}

// canRedirectStdout reports whether the output streams can be redirected in
// place. It is false on platforms without a dup2 equivalent, where the caller
// must capture output through a subprocess pipe instead.
func canRedirectStdout() bool { return true }
