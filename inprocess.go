package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
)

// ExecuteArgsFunc is a tool's in-process entry point. It runs one command with
// an explicit argument vector and returns the process exit code, without
// calling os.Exit.
//
// This is the seam the TUI prefers: no process spawn, and the execution runs
// inside the tool's own binary, so it exercises the same code path a shell
// invocation would.
type ExecuteArgsFunc func(ctx context.Context, args []string) int

// InProcessRunner invokes a tool's own ExecuteArgs, capturing its event
// stream in the same pipe a shell pipeline would have produced.
//
// It works by pointing the --events flag at stdout and temporarily swapping
// os.Stdout for the write end of a pipe. That is the tool's own documented
// contract -- `--events stdout` emits JSONL on stdout and routes report text
// to stderr -- so no repository needs its event layer modified to support the
// TUI. Because stdout carries only JSONL, the transcript cannot be polluted by
// the tool's human-readable output, and the TUI still never parses prose.
//
// The swap is safe because the TUI runs exactly one command at a time on its
// own goroutine while the render loop is the only other consumer of the
// process. Concurrent invocations are not supported, and the mutex makes that
// explicit rather than letting it corrupt the stream.
type InProcessRunner struct {
	// Execute is the tool's in-process entry point.
	Execute ExecuteArgsFunc
	// ToolName is the header name.
	ToolName string
	// Meta supplies completion metadata.
	Meta []Command
	// Prefix is prepended to every argument vector, for flags the tool
	// requires on all invocations.
	Prefix []string
	// EventFlag and EventValue form the flag used to route JSONL to stdout.
	// Tools whose flag is spelled differently can override these; the
	// defaults match the shared --events contract.
	EventFlag  string
	EventValue string
}

func (r *InProcessRunner) Name() string { return r.ToolName }

func (r *InProcessRunner) Commands() []Command { return r.Meta }

func (r *InProcessRunner) Run(ctx context.Context, args []string, events io.Writer) (int, error) {
	// Serialise execution: two concurrent runs would both be writing to
	// os.Stdout and the captured stream would interleave into nonsense.
	var mu sync.Mutex
	mu.Lock()
	defer mu.Unlock()

	pr, pw, err := os.Pipe()
	if err != nil {
		return 1, fmt.Errorf("creating event pipe: %w", err)
	}

	// The tool writes to os.Stdout; the TUI reads pr. The write end is
	// buffered so a tool emitting events faster than the UI renders does not
	// block on a full pipe.
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(events, pr)
		close(done)
	}()

	saved := os.Stdout
	os.Stdout = pw

	full := append([]string{}, r.Prefix...)
	full = append(full, r.eventFlag()...)
	full = append(full, args...)

	// The tool must not inherit a cancelled context from a previous run, and
	// it needs a context that is cancelled when ctx is, so Ctrl+C stops the
	// work rather than merely the UI.
	code := r.Execute(ctx, full)

	os.Stdout = saved
	// Closing the write end is what lets the copy goroutine observe EOF.
	_ = pw.Close()
	wg.Wait()
	_ = pr.Close()
	<-done

	if ctx.Err() != nil && code == 0 {
		// The tool finished cleanly despite the interrupt; report the
		// interrupt, since that is what the user asked for.
		return ExitCancelled, nil
	}
	return code, nil
}

func (r *InProcessRunner) eventFlag() []string {
	flag := r.EventFlag
	if flag == "" {
		flag = "--events"
	}
	value := r.EventValue
	if value == "" {
		value = "stdout"
	}
	return []string{flag, value}
}
