//go:build windows

package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
)

func osExecutable() (string, error) { return os.Executable() }

type child struct {
	cmd *exec.Cmd
	r   *os.File
}

func startChild(ctx context.Context, path string, args []string, stderr io.Writer) (*child, error) {
	_ = ctx
	cmd := exec.Command(path, args...)

	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("creating event pipe: %w", err)
	}
	cmd.Stdout = pw
	cmd.Stderr = stderr
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		_ = pr.Close()
		_ = pw.Close()
		return nil, fmt.Errorf("starting %s: %w", path, err)
	}
	_ = pw.Close()
	return &child{cmd: cmd, r: pr}, nil
}

func (c *child) wait(ctx context.Context) int {
	done := make(chan error, 1)
	go func() { done <- c.cmd.Wait() }()

	select {
	case err := <-done:
		return exitStatus(err)
	case <-ctx.Done():
	}

	// os.Interrupt is not implemented for Windows processes, so there is no
	// equivalent of SIGINT. Terminating is the closest available behaviour
	// and the execution is still reported as cancelled.
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	<-done
	return ExitCancelled
}

func exitStatus(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if code := ee.ExitCode(); code >= 0 {
			return code
		}
	}
	return 1
}

func signalChild(p *os.Process) error {
	if p == nil {
		return nil
	}
	return p.Kill()
}

func killChild(p *os.Process) error {
	if p == nil {
		return nil
	}
	return p.Kill()
}
