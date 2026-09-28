package tui

import (
	"fmt"
	"io"
	"os"
	"sync"
)

// Capture redirects the process's standard output and standard error into dst
// for the duration of fn, then puts both back.
//
// It exists because a tool's own output has to reach the transcript, and
// reassigning os.Stdout is not enough to guarantee that. A package that
// captured the stream into a variable during initialisation keeps writing to
// the original descriptor however the variable is later assigned, so its output
// escapes the capture and draws over the interface's own frames. Moving the
// descriptor catches every writer, including one the tool never thinks about.
//
// The capture is at the descriptor level and process-wide, so it is only safe
// while one command runs at a time with no other consumer of the terminal. The
// TUI's render loop is drawn from a private duplicate of the original terminal,
// which is what lets it keep redrawing while a capture is in effect.
//
// Both streams are captured into the one writer. A tool writes much of what the
// operator asked for to standard error -- a capability table, a warning, the
// reason a run failed -- so capturing only stdout would leave that text landing
// on the real terminal. The TUI tells the two apart by shape, not by which
// descriptor they arrived on: a line beginning with "{" that parses as an event
// envelope is structured, anything else is the tool's own output.
//
// If fn returns an error, it is returned unchanged, and the streams are always
// restored even then: leaving standard error pointed at a dead pipe would make
// the next write fail, turning one failed command into a broken process.
func Capture(dst io.Writer, fn func() error) error {
	// Serialise: two overlapping captures would interleave into the same pipe
	// and hand each caller a half-foreign transcript.
	var mu sync.Mutex
	mu.Lock()
	defer mu.Unlock()

	if dst == nil {
		return fmt.Errorf("tui: capture needs an output writer")
	}

	pr, pw, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("tui: creating capture pipe: %w", err)
	}

	// Buffer the write end: a tool that emits faster than the interface reads
	// must not block on a full pipe, which would look like a hung command.
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(dst, pr)
		close(done)
	}()

	restoreOut, err := redirectStdout(pw)
	if err != nil {
		_ = pw.Close()
		<-done
		_ = pr.Close()
		return fmt.Errorf("tui: cannot capture tool output: %w", err)
	}
	restoreErr, err := redirectStderr(pw)
	if err != nil {
		// Undo the first half before giving up, or the tool's output would keep
		// going to a pipe nobody is reading.
		restoreOut()
		_ = pw.Close()
		<-done
		_ = pr.Close()
		return fmt.Errorf("tui: cannot capture tool diagnostics: %w", err)
	}

	fnErr := fn()

	restoreErr()
	restoreOut()
	// Closing the write end is what lets the copy goroutine observe EOF.
	_ = pw.Close()
	wg.Wait()
	_ = pr.Close()
	<-done

	return fnErr
}
