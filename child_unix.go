//go:build !windows

package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func osExecutable() (string, error) { return os.Executable() }

// child is a running tool process whose stdout is the event stream.
type child struct {
	cmd *exec.Cmd
	r   *os.File
}

// startChild launches the tool with stdout connected to a pipe the TUI reads
// as its event stream.
//
// The child is placed in its own process group (Setpgid) so that signalling it
// cannot reach the TUI itself, and so a tool that shells out to helpers
// interrupts the whole subtree rather than leaving orphans behind.
//
// exec.CommandContext is deliberately not used: it SIGKILLs on context
// cancellation, which would race ahead of the tool's own cleanup and lose the
// partial artifacts that cancellation is supposed to preserve.
func startChild(ctx context.Context, path string, args []string, stderr io.Writer) (*child, error) {
	_ = ctx
	cmd := exec.Command(path, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("creating event pipe: %w", err)
	}
	cmd.Stdout = pw
	// The child's human-readable output is the TUI's business, not the event
	// stream's. Sending it to stderr keeps it out of the pipe, which must
	// carry JSONL and nothing else.
	cmd.Stderr = stderr
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		_ = pr.Close()
		_ = pw.Close()
		return nil, fmt.Errorf("starting %s: %w", path, err)
	}
	// The parent must drop the write end immediately: while the parent holds
	// a copy, the reading goroutine never sees EOF, because the pipe's
	// reference count includes the parent's descriptor.
	_ = pw.Close()
	return &child{cmd: cmd, r: pr}, nil
}

// wait blocks until the child exits and returns its exit status.
//
// If ctx is cancelled first, the child is sent SIGINT and given interruptGrace
// to finish; only then is it killed. A cancelled execution always reports
// ExitCancelled, because the reason it stopped was the interrupt, not whatever
// status its cleanup path happened to produce.
func (c *child) wait(ctx context.Context) int {
	done := make(chan error, 1)
	go func() { done <- c.cmd.Wait() }()

	select {
	case err := <-done:
		return exitStatus(err)
	case <-ctx.Done():
	}

	_ = signalChild(c.cmd.Process)
	select {
	case <-done:
		return ExitCancelled
	case <-time.After(interruptGrace):
	}

	// The child ignored SIGINT. It may be stuck in a syscall, so give up on
	// the graceful path and take it out.
	_ = killChild(c.cmd.Process)
	<-done
	return ExitCancelled
}

// exitStatus maps a wait error onto a shell exit status.
func exitStatus(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if code := ee.ExitCode(); code >= 0 {
			return code
		}
		// Killed by a signal: ExitCode reports -1, but the shell convention
		// is 128+signal, and SIGINT (2) is 130.
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal())
		}
	}
	return 1
}

// signalChild delivers SIGINT to the child's process group.
func signalChild(p *os.Process) error {
	if p == nil {
		return nil
	}
	// A negative pid targets the process group created by Setpgid, reaching
	// any helpers the tool spawned.
	if err := syscall.Kill(-p.Pid, syscall.SIGINT); err != nil {
		return p.Signal(syscall.SIGINT)
	}
	return nil
}

func killChild(p *os.Process) error {
	if p == nil {
		return nil
	}
	if err := syscall.Kill(-p.Pid, syscall.SIGKILL); err != nil {
		return p.Kill()
	}
	return nil
}
