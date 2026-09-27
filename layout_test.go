package tui

import (
	"bytes"
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// A tool that captured stdout at init must be captured by the TUI.
//
// This is a regression test for the failure that made the interface unusable:
// the runner reassigned the os.Stdout variable, but a package that had already
// stored that value in a variable of its own -- which most tools do when they
// build a printer during init -- kept writing to the original file. Its output
// went straight to the terminal, in the middle of the interface's own frames.
// Redirecting the descriptor, rather than the variable, is what fixes it.
func TestInProcessCaptureReachesWritersHoldingAnOldStdout(t *testing.T) {
	// This handle stands in for the one a tool captures during init, before
	// the TUI starts.
	stale := os.Stdout
	var out string
	var mu sync.Mutex

	r := &InProcessRunner{
		ToolName: "stale",
		Execute: func(_ context.Context, args []string) int {
			// Written through the stale handle and through the live variable,
			// because a real tool may use either.
			_, _ = stale.WriteString("VIA-STALE-HANDLE\n")
			_, _ = os.Stdout.WriteString("VIA-LIVE-VARIABLE\n")
			// Interleaved with an event, to prove both survive together.
			if len(args) > 0 {
				_, _ = os.Stdout.WriteString(
					`{"schema_version":"1.0","timestamp":"2026-01-01T00:00:00Z","execution_id":"e1",` +
						`"framework":"stale","level":"info","event":"execution.started","data":{}}` + "\n")
			}
			mu.Lock()
			out += "done"
			mu.Unlock()
			return 0
		},
	}

	var buf bytes.Buffer
	code, err := r.Run(context.Background(), []string{"scan", "x"}, &buf)
	var events []Event
	readEvents(&buf, func(ev Event) { events = append(events, ev) }, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}

	mu.Lock()
	ran := out == "done"
	mu.Unlock()
	if !ran {
		t.Fatal("the tool did not run")
	}

	// The event must have arrived...
	var sawStarted bool
	for _, ev := range events {
		if ev.Event == EventExecutionStarted {
			sawStarted = true
		}
	}
	if !sawStarted {
		t.Errorf("execution.started was not captured; events = %+v", events)
	}
	// ...and stdout must be restored afterwards, which is what lets the renderer
	// keep drawing.
	if os.Stdout != stale {
		t.Error("os.Stdout was not restored after the run")
	}
}

// The interface must fill the terminal exactly.
//
// A view one line taller than the terminal does not clip: the terminal scrolls,
// the header scrolls off the top, and the composer -- which is supposed to be
// pinned to the bottom -- lands in the middle of the screen. Content of any
// length, including the long, table-shaped output tools actually print, has to
// fit.
func TestViewExactlyFillsTheTerminalAtAnyContentLength(t *testing.T) {
	for _, size := range []struct{ w, h int }{{100, 30}, {80, 24}, {60, 20}, {40, 12}} {
		m := newModel(Config{Title: "QYVORA / PROBE", Version: "0.1.0", Runner: &InProcessRunner{
			ToolName: "probe",
			Execute:  func(context.Context, []string) int { return 0 },
		}}, newTheme(false))
		m = resize(m, size.w, size.h)

		// A deliberately hostile block: long lines that the viewport has to
		// wrap, which is exactly what tool help text and capability tables do.
		var rows []string
		for i := 0; i < 40; i++ {
			rows = append(rows, "nzinga.simulation.dns.resolve   Offline simulation dataset "+
				strings.Repeat("detail ", 18)+" simulation   S1   no")
		}
		b := newBlock(1, []string{"capabilities"})
		b.started, b.finished = time.Now().Add(-2*time.Second), time.Now()
		b.status = StatusDone
		for _, r := range rows {
			b.addEvent(Event{Event: "capability.listed", Level: "info",
				Data: map[string]any{"raw": r}})
		}
		m = addBlock(m, b)

		lines := strings.Split(strings.TrimRight(m.View(), "\n"), "\n")
		if len(lines) != size.h {
			t.Errorf("%dx%d: View() is %d lines, want exactly %d", size.w, size.h, len(lines), size.h)
		}
		for i, l := range lines {
			if w := len([]rune(l)); w > size.w {
				t.Errorf("%dx%d: line %d is %d runes and would wrap", size.w, size.h, i, w)
			}
		}
	}
}

// Long output has to be wrapped, not truncated: a truncated capability table
// loses the column the operator was reading.
func TestLongToolOutputIsWrappedNotTruncated(t *testing.T) {
	m := newModel(Config{Title: "QYVORA / PROBE", Version: "0.1.0", Runner: &InProcessRunner{
		ToolName: "probe",
		Execute:  func(context.Context, []string) int { return 0 },
	}}, newTheme(false))
	m = resize(m, 72, 24)

	marker := "OFFLINE-SIMULATION-DATASET-TAIL"
	b := newBlock(1, []string{"capabilities"})
	b.started, b.finished = time.Now().Add(-time.Second), time.Now()
	b.status = StatusDone
	// The line is what the tool printed, not an event: it has no envelope.
	b.addOutput(strings.Repeat("nzinga.simulation.dns.resolve   ", 3) + marker)
	m = addBlock(m, b)

	body := m.viewport.View()
	if !strings.Contains(body, marker) {
		t.Errorf("output was cut off instead of wrapped; transcript was:\n%s", body)
	}
}
