package tui

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// A writer that captured the stream during package initialisation keeps writing
// to the original descriptor, so a capture that only reassigns os.Stdout
// silently misses it and its text draws over the interface. Capture moves the
// descriptor, so it has to catch even that.
func TestCaptureReachesAWriterHoldingAnOldStdout(t *testing.T) {
	// Taken before the capture, exactly as a package-level var would.
	stale := os.Stdout

	var buf bytes.Buffer
	var duringStdout string
	var duringStderr string

	err := Capture(&buf, func() error {
		_, _ = stale.WriteString("through the stale handle\n")
		_, _ = os.Stdout.WriteString("through os.Stdout\n")
		_, _ = os.Stderr.WriteString("through os.Stderr\n")
		// The variable must be redirected inside the capture too, for tools
		// that write through it rather than the descriptor.
		_, _ = os.Stdout.WriteString(duringStdout)
		_, _ = os.Stderr.WriteString(duringStderr)
		duringStdout = ""
		duringStderr = ""
		return nil
	})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}

	got := buf.String()
	for _, want := range []string{
		"through the stale handle",
		"through os.Stdout",
		"through os.Stderr",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("capture lost %q; got:\n%s", want, got)
		}
	}
}

// Both descriptors must be usable again afterwards. A capture that leaked a
// redirection would make the next write fail, turning one command into a broken
// process.
func TestCaptureRestoresBothStreams(t *testing.T) {
	var buf bytes.Buffer
	if err := Capture(&buf, func() error { return nil }); err != nil {
		t.Fatalf("Capture: %v", err)
	}

	if !ttyLike(t, os.Stdout) {
		t.Error("stdout was not restored after Capture")
	}
	if !ttyLike(t, os.Stderr) {
		t.Error("stderr was not restored after Capture")
	}

	// A write must succeed and must not land in the finished capture.
	if _, err := os.Stderr.WriteString("after capture\n"); err != nil {
		t.Errorf("writing to stderr after Capture: %v", err)
	}
	if strings.Contains(buf.String(), "after capture") {
		t.Error("stderr still pointed at the capture pipe after Capture returned")
	}
}

// ttyLike reports whether f is still the process's real stream rather than a
// pipe left over from a capture.
func ttyLike(t *testing.T, f *os.File) bool {
	t.Helper()
	_, err := f.Stat()
	return err == nil
}

// An error from fn must reach the caller, and the streams must be restored
// anyway: a failed command must not leave the process writing into a dead pipe.
func TestCaptureReturnsErrorAndStillRestores(t *testing.T) {
	var buf bytes.Buffer
	sentinel := os.ErrClosed
	err := Capture(&buf, func() error { return sentinel })
	if err != sentinel {
		t.Fatalf("Capture returned %v, want the error fn returned", err)
	}
	if _, statErr := os.Stderr.Stat(); statErr != nil {
		t.Errorf("stderr not restored after a failing capture: %v", statErr)
	}
}

// Capture must not report completion before everything the tool wrote has been
// read out of the pipe. A tool that emits a burst and returns would otherwise
// lose its tail, which is the case a user notices: the last findings of a scan.
func TestCaptureDrainsOutputWrittenBeforeItReturns(t *testing.T) {
	var buf bytes.Buffer
	line := strings.Repeat("payload ", 200)
	const lines = 200
	err := Capture(&buf, func() error {
		for i := 0; i < lines; i++ {
			if _, werr := os.Stdout.WriteString(line + "\n"); werr != nil {
				return werr
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	// Count lines, not occurrences of the word: the filler repeats it, so a
	// substring count would be checking the filler length rather than the
	// number of lines that survived.
	if got := strings.Count(buf.String(), "\n"); got != lines {
		t.Errorf("captured %d lines, want %d", got, lines)
	}
}

// A nil destination is a programming error, and it must be reported rather than
// panicking inside a goroutine the caller cannot see.
func TestCaptureRejectsNilWriter(t *testing.T) {
	if err := Capture(nil, func() error { return nil }); err == nil {
		t.Error("Capture(nil) returned nil error, want a failure")
	}
}

func TestCaptureRestoresQuicklyForManyRuns(t *testing.T) {
	// A session runs one capture per command; leaking a descriptor per command
	// would exhaust the process's file table over a long session.
	for i := 0; i < 50; i++ {
		var buf bytes.Buffer
		if err := Capture(&buf, func() error {
			_, _ = os.Stdout.WriteString("x\n")
			return nil
		}); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		time.Sleep(time.Millisecond)
	}
}

// The mutex that serialises captures has to outlive the call, or two concurrent
// captures both install their pipe and the later one wins: every writer lands in
// the same transcript and the other caller sees nothing but foreign output. This
// is the failure the comment claims to prevent, so it is worth a test that fails
// when the lock is function-local.
func TestCaptureSerialisesOverlappingCalls(t *testing.T) {
	const callers = 4

	var (
		buffers = make([]bytes.Buffer, callers)
		start   = make(chan struct{})
		wg      sync.WaitGroup
	)
	for i := range callers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Line up every goroutine so the captures genuinely overlap
			// rather than happening to run one after another.
			<-start
			_ = Capture(&buffers[i], func() error {
				for n := range 20 {
					fmt.Fprintf(os.Stdout, "caller%d-line%d\n", i, n)
					runtime.Gosched()
				}
				return nil
			})
		}(i)
	}
	close(start)
	wg.Wait()

	for i := range callers {
		got := buffers[i].String()
		if got == "" {
			t.Errorf("caller %d captured nothing: its transcript went to another caller", i)
			continue
		}
		for _, line := range strings.Split(strings.TrimSpace(got), "\n") {
			if want := fmt.Sprintf("caller%d-line", i); !strings.HasPrefix(line, want) {
				t.Fatalf("caller %d transcript contains foreign line %q", i, line)
			}
		}
	}
}
