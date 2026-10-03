//go:build unix && !aix && !darwin && !dragonfly && !freebsd && !netbsd && !openbsd && !android && !(linux && !arm64 && !riscv64 && !loong64)

package tui

import (
	"errors"
	"os"
)

// These platforms are unix but have no syscall.Dup2. Linux dropped the legacy
// dup2 syscall on arm64, riscv64 and loong64 in favour of dup3, and solaris
// never had it; Go's syscall package reflects that exactly, so referring to
// syscall.Dup2 on them is a compile error rather than a runtime one.
//
// The fix is not to reach for dup3 or for cgo. It is to say so plainly, because
// canRedirectStdout already exists to tell the caller to capture output through a
// subprocess pipe instead. That path works everywhere and is what these
// platforms get.
var errNoDup2 = errors.New("this platform has no dup2, so the output streams cannot be redirected in place")

func redirectStdout(w *os.File) (restore func(), err error) {
	return nil, errNoDup2
}

func redirectStderr(w *os.File) (restore func(), err error) {
	return nil, errNoDup2
}

func canRedirectStdout() bool { return false }
