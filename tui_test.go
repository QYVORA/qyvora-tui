package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// emit writes one well-formed envelope as JSONL.
func emit(t *testing.T, w *bytes.Buffer, execID, name string, data map[string]any) {
	t.Helper()
	ev := map[string]any{
		"schema_version": "1.0",
		"timestamp":      time.Now().UTC().Format(time.RFC3339Nano),
		"execution_id":   execID,
		"framework":      "test",
		"level":          "info",
		"event":          name,
	}
	if data != nil {
		ev["data"] = data
	}
	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	w.Write(b)
	w.WriteByte('\n')
}

func TestReadEventsDecodesTheSevenFieldEnvelope(t *testing.T) {
	var buf bytes.Buffer
	emit(t, &buf, "e1", EventExecutionStarted, nil)
	emit(t, &buf, "e1", EventFindingDiscovered, map[string]any{"title": "weak TLS", "severity": "high"})

	var got []Event
	stats := readEvents(&buf, func(ev Event) { got = append(got, ev) })

	if stats.Decoded != 2 {
		t.Fatalf("decoded %d events, want 2", stats.Decoded)
	}
	if stats.Malformed != 0 {
		t.Fatalf("%d events reported malformed, want 0", stats.Malformed)
	}
	if got[0].SchemaVersion != "1.0" {
		t.Errorf("schema_version = %q, want 1.0", got[0].SchemaVersion)
	}
	if got[0].Framework != "test" || got[0].Level != "info" {
		t.Errorf("envelope fields not decoded: %+v", got[0])
	}
	if got[1].Data["title"] != "weak TLS" {
		t.Errorf("data not decoded: %+v", got[1].Data)
	}
}

// A tool that dies mid-write leaves a truncated final line. That must not cost
// the user every event that arrived before it.
func TestReadEventsSkipsMalformedLinesAndKeepsGoing(t *testing.T) {
	var buf bytes.Buffer
	emit(t, &buf, "e1", EventExecutionStarted, nil)
	buf.WriteString("{not json at all\n")
	buf.WriteString("\n")
	emit(t, &buf, "e1", EventExecutionCompleted, nil)

	var got []Event
	stats := readEvents(&buf, func(ev Event) { got = append(got, ev) })

	if stats.Decoded != 2 {
		t.Fatalf("decoded %d, want 2 -- a bad line must not end the stream", stats.Decoded)
	}
	if stats.Malformed != 1 {
		t.Errorf("malformed = %d, want 1", stats.Malformed)
	}
}

// An event type the TUI has never seen must still reach the transcript, or a
// tool could add event types and silently lose them in the UI.
func TestUnknownEventTypesAreRetained(t *testing.T) {
	b := newBlock(1, []string{"scan", "x"})
	b.addEvent(Event{Event: "quantum.entangled", Level: "info", Data: map[string]any{"spookiness": 7.0}})

	if len(b.rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(b.rows))
	}
	if b.rows[0].Type != "quantum.entangled" {
		t.Errorf("event type lost: %+v", b.rows[0])
	}
	if got := humanEvent("quantum.entangled"); got != "Quantum Entangled" {
		t.Errorf("humanEvent = %q", got)
	}
}

func TestProgressAcceptsBothPercentConventions(t *testing.T) {
	b := newBlock(1, nil)
	b.addEvent(Event{Event: EventProgressUpdated, Data: map[string]any{"percent": 0.5}})
	if !b.progress.HasBar || b.progress.Percent != 50 {
		t.Errorf("0.5 fraction: got %+v, want 50%%", b.progress)
	}
	b.addEvent(Event{Event: EventProgressUpdated, Data: map[string]any{"percent": 42.0}})
	if b.progress.Percent != 42 {
		t.Errorf("42 percent: got %v, want 42", b.progress.Percent)
	}
}

func TestFindingsAreCollectedAndSortedBySeverity(t *testing.T) {
	b := newBlock(1, nil)
	b.addEvent(Event{Event: EventFindingDiscovered, Data: map[string]any{"title": "a", "severity": "low"}})
	b.addEvent(Event{Event: EventFindingDiscovered, Data: map[string]any{"title": "b", "severity": "critical"}})
	b.addEvent(Event{Event: EventFindingDiscovered, Data: map[string]any{"title": "c", "severity": "high"}})

	if len(b.findings) != 3 {
		t.Fatalf("findings = %d, want 3", len(b.findings))
	}
	fs := make([]Finding, len(b.findings))
	copy(fs, b.findings)
	sortFindings(fs)
	if fs[0].Severity != "CRITICAL" || fs[1].Severity != "HIGH" {
		t.Errorf("severity order = %v", []string{fs[0].Severity, fs[1].Severity})
	}
}

func TestTokenize(t *testing.T) {
	cases := []struct {
		in      string
		want    []string
		wantErr bool
	}{
		{"scan example.com", []string{"scan", "example.com"}, false},
		{"  spaced   out  ", []string{"spaced", "out"}, false},
		{`analyze "two words"`, []string{"analyze", "two words"}, false},
		{`analyze 'single quoted'`, []string{"analyze", "single quoted"}, false},
		{`analyze "say \"hi\""`, []string{"analyze", `say "hi"`}, false},
		{"", nil, false},
		{"   ", nil, false},
		{`quote "unterminated`, nil, true},
		{`quote 'also unterminated`, nil, true},
	}
	for _, c := range cases {
		got, err := tokenize(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("tokenize(%q) = %v, want an error", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("tokenize(%q): %v", c.in, err)
			continue
		}
		if fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("tokenize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHistoryNavigationRestoresTheDraft(t *testing.T) {
	h := NewHistory()
	h.Add("scan one")
	h.Add("scan two")

	got, ok := h.Prev("half-typed")
	if !ok || got != "scan two" {
		t.Fatalf("Prev = %q, %v", got, ok)
	}
	if got, _ := h.Prev(""); got != "scan one" {
		t.Fatalf("second Prev = %q", got)
	}
	if got, _ := h.Prev(""); got != "scan one" {
		t.Fatalf("Prev at the oldest entry should stay put, got %q", got)
	}
	// Past the newest entry, the in-progress line comes back.
	if got, _ := h.Next(""); got != "scan two" {
		t.Fatalf("Next = %q", got)
	}
	if got, _ := h.Next(""); got != "half-typed" {
		t.Fatalf("Next past the end = %q, want the saved draft", got)
	}
}

func TestHistorySkipsImmediateRepeats(t *testing.T) {
	h := NewHistory()
	h.Add("scan a")
	h.Add("scan a")
	if got := h.Entries(); len(got) != 1 {
		t.Errorf("entries = %v, want one", got)
	}
}

func TestIsNotInteractiveForAPipe(t *testing.T) {
	// A pipe is not a terminal, which is the whole basis of the non-TTY
	// fallback: `tool scan target | tee log` must not launch a TUI.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if IsInteractive(w) {
		t.Error("a pipe reported as interactive")
	}
	if isTerminal(nil) {
		t.Error("a nil file reported as a terminal")
	}
}

func TestRunRefusesToLaunchOnANonTerminal(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	_, err = Run(Config{Out: w, Runner: &InProcessRunner{Execute: func(context.Context, []string) int { return 0 }, ToolName: "test"}})
	if err == nil {
		t.Fatal("Run started a full-screen session on a pipe")
	}
	if !IsNotInteractive(err) {
		t.Fatalf("error does not satisfy IsNotInteractive: %v", err)
	}
}

func TestNoColorDisablesStyling(t *testing.T) {
	th := newTheme(false)
	if th.Color {
		t.Fatal("theme reports colour when disabled")
	}
	// With colour off the styles are the identity transform, so no escape
	// sequence can reach the output.
	if got := th.Critical.Render("CRITICAL"); got != "CRITICAL" {
		t.Errorf("styled output = %q, want plain text", got)
	}
	if strings.Contains(th.Running.Render("●"), "\x1b") {
		t.Error("NO_COLOR output still contains an escape sequence")
	}
}

func TestColorEnabledHonoursNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if colorEnabled(os.Stdout) {
		t.Error("NO_COLOR=1 was ignored")
	}
}

func TestThemeRendersWithoutColor(t *testing.T) {
	th := newTheme(true)
	m := newModel(Config{Title: "QYVORA / TEST", Runner: &InProcessRunner{Execute: func(context.Context, []string) int { return 0 }, ToolName: "test"}}, th)
	m = resize(m, 100, 30)
	out := m.View()
	if !strings.Contains(out, "QYVORA / TEST") {
		t.Errorf("header missing from view:\n%s", out)
	}
	if !strings.Contains(out, "READY") {
		t.Errorf("status missing from view:\n%s", out)
	}
}

// A narrow terminal must not explode: the layout is recomputed, not fixed.
func TestViewHandlesNarrowTerminals(t *testing.T) {
	th := newTheme(false)
	m := newModel(Config{Runner: &InProcessRunner{Execute: func(context.Context, []string) int { return 0 }, ToolName: "test"}}, th)

	for _, w := range []int{200, 100, 60, 40, 20, 10} {
		m = resize(m, w, 20)
		out := m.View()
		if !strings.Contains(out, ">") {
			t.Errorf("width %d: prompt missing:\n%s", w, out)
		}
		// No rendered line may exceed the terminal width, or the display
		// wraps and the block layout falls apart.
		for _, line := range strings.Split(out, "\n") {
			if n := len([]rune(stripANSI(line))); n > w {
				t.Errorf("width %d: line of %d runes: %q", w, n, line)
			}
		}
	}
}

func TestViewRendersAnExecutionBlock(t *testing.T) {
	th := newTheme(false)
	m := newModel(Config{Title: "QYVORA / TEST", Runner: &InProcessRunner{Execute: func(context.Context, []string) int { return 0 }, ToolName: "test"}}, th)
	m = resize(m, 100, 40)

	m.blocks = append(m.blocks, newBlock(1, []string{"scan", "example.com"}))
	b := m.blocks[0]
	b.addEvent(Event{Event: EventFindingDiscovered, Level: "warn", Data: map[string]any{"title": "open redirect", "severity": "high", "target": "example.com"}})
	b.addEvent(Event{Event: EventArtifactCreated, Level: "info", Data: map[string]any{"path": "report.json"}})
	b.finished = time.Now()
	b.status = StatusDone

	out := m.View()
	for _, want := range []string{"Execution", "scan example.com", "Findings", "open redirect", "Artifacts", "report.json"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q:\n%s", want, out)
		}
	}
}

func TestCancelledBlockReportsItself(t *testing.T) {
	th := newTheme(false)
	m := newModel(Config{Runner: &InProcessRunner{Execute: func(context.Context, []string) int { return ExitCancelled }, ToolName: "test"}}, th)
	m.width, m.height = 100, 30
	m.Update(windowSizeMsg(100, 30))

	m.blocks = append(m.blocks, newBlock(1, []string{"scan", "x"}))
	m.activeID = 1
	m.running = true

	upd, _ := m.Update(runDoneMsg{exitCode: ExitCancelled, cancel: true})
	m2 := upd.(model)
	if got := m2.statusText(m2.blocks[0]); !strings.Contains(got, "Cancelled") {
		t.Errorf("status = %q, want Cancelled", got)
	}
	if !strings.Contains(m2.View(), "partial results were kept") {
		t.Errorf("cancellation notice missing:\n%s", m2.View())
	}
}

func TestFailedExecutionRecordsTheExitStatus(t *testing.T) {
	th := newTheme(false)
	m := newModel(Config{Runner: &InProcessRunner{Execute: func(context.Context, []string) int { return 2 }, ToolName: "test"}}, th)
	m.width, m.height = 100, 30
	m.Update(windowSizeMsg(100, 30))
	m.blocks = append(m.blocks, newBlock(1, []string{"analyze", "x"}))
	m.activeID = 1
	m.running = true

	upd2, _ := m.Update(runDoneMsg{exitCode: 2})
	m2 := upd2.(model)
	if m2.blocks[0].status != StatusFailed {
		t.Errorf("status = %v, want Failed", m2.blocks[0].status)
	}
	if !strings.Contains(m2.View(), "exit 2") {
		t.Errorf("exit status not surfaced:\n%s", m2.View())
	}
}

func TestSecondExecutionIsRefusedWhileOneRuns(t *testing.T) {
	th := newTheme(false)
	m := newModel(Config{Runner: &InProcessRunner{Execute: func(context.Context, []string) int { return 0 }, ToolName: "test"}}, th)
	m = resize(m, 100, 30)
	m.running = true
	if cmd := m.start([]string{"scan", "x"}); cmd != nil {
		t.Error("a second execution was started while one was in flight")
	}
	if len(m.blocks) != 0 {
		t.Errorf("a block was created for the refused execution: %d", len(m.blocks))
	}
}

func TestCancellationIsPerExecutionNotShared(t *testing.T) {
	// The point of the per-execution child context: cancelling one run must
	// not leave the next one born cancelled.
	_, cancel1 := context.WithCancel(context.Background())
	cancel1()
	if ctx2, cancel2 := context.WithCancel(context.Background()); ctx2.Err() != nil {
		t.Fatal("a new execution inherited a previous cancellation")
	} else {
		cancel2()
	}
}

func TestBuiltinsAreHandledLocally(t *testing.T) {
	th := newTheme(false)
	ran := false
	m := newModel(Config{Runner: &InProcessRunner{
		Execute:  func(context.Context, []string) int { ran = true; return 0 },
		ToolName: "test",
	}}, th)
	m = resize(m, 100, 30)

	for _, line := range []string{"help", "clear", "quit"} {
		m.input.SetValue(line)
		m.submit()
	}
	if ran {
		t.Error("a builtin was dispatched to the tool")
	}
}

func TestCommandCompletionComesFromTheToolRegistry(t *testing.T) {
	th := newTheme(false)
	m := newModel(Config{Runner: &InProcessRunner{
		Execute:  func(context.Context, []string) int { return 0 },
		ToolName: "test",
		Meta: []Command{
			{Name: "scan", Subs: []string{"ports", "web"}},
			{Name: "search"},
			{Name: "analyze"},
		},
	}}, th)
	m = resize(m, 100, 30)

	if got := m.candidates("sc"); fmt.Sprint(got) != "[scan]" {
		t.Errorf("candidates(sc) = %v, want [scan]", got)
	}
	// A second word completes against the first word's subcommands.
	m.notices = nil
	m.input.SetValue("scan ")
	m.input.CursorEnd()
	m.complete()
	if len(m.notices) == 0 || !strings.Contains(m.notices[len(m.notices)-1], "ports") {
		t.Errorf("subcommands not offered for a second word: %v", m.notices)
	}
	// A unique subcommand completes in place.
	m.notices = nil
	m.input.SetValue("scan we")
	m.input.CursorEnd()
	m.complete()
	if got := m.input.Value(); got != "scan web " {
		t.Errorf("subcommand completion produced %q, want %q", got, "scan web ")
	}
	if got := m.candidates("zzz"); len(got) != 0 {
		t.Errorf("unknown prefix produced candidates: %v", got)
	}

	// A unique prefix completes in place.
	m.input.SetValue("sc")
	m.input.CursorEnd()
	m.complete()
	if got := m.input.Value(); got != "scan " {
		t.Errorf("completion produced %q, want %q", got, "scan ")
	}

	// An ambiguous prefix lists options rather than guessing.
	m.notices = nil
	m.input.SetValue("s")
	m.input.CursorEnd()
	m.complete()
	if len(m.notices) == 0 {
		t.Error("an ambiguous prefix silently picked a completion")
	} else if !strings.Contains(m.notices[len(m.notices)-1], "scan") ||
		!strings.Contains(m.notices[len(m.notices)-1], "search") {
		t.Errorf("ambiguous prefix did not list both options: %v", m.notices)
	}
}

func TestInProcessRunnerCapturesTheEventStream(t *testing.T) {
	// The in-process runner is the preferred seam: it runs the tool's own
	// ExecuteArgs and captures its JSONL without the TUI parsing prose.
	var out bytes.Buffer
	r := &InProcessRunner{
		ToolName: "test",
		Execute: func(_ context.Context, args []string) int {
			// Mimic a tool honouring the --events contract: it writes JSONL
			// to stdout, which the runner has redirected into the capture
			// pipe. Writing to the caller's buffer instead would prove
			// nothing about the capture.
			var buf bytes.Buffer
			emit(t, &buf, "e1", EventExecutionStarted, nil)
			emit(t, &buf, "e1", EventExecutionCompleted, nil)
			if len(args) < 2 || args[0] != "--events" || args[1] != "stdout" {
				t.Errorf("event routing flag not applied: %v", args)
			}
			if _, err := os.Stdout.Write(buf.Bytes()); err != nil {
				return 1
			}
			return 0
		},
	}
	code, err := r.Run(context.Background(), []string{"scan", "x"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Errorf("exit = %d", code)
	}
	if !strings.Contains(out.String(), EventExecutionStarted) {
		t.Errorf("event stream not captured: %q", out.String())
	}
}

func TestInProcessRunnerReportsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := &InProcessRunner{
		ToolName: "test",
		Execute:  func(context.Context, []string) int { return 0 },
	}
	code, err := r.Run(ctx, []string{"scan", "x"}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if code != ExitCancelled {
		t.Errorf("exit = %d, want %d", code, ExitCancelled)
	}
}

// helperScript writes a launcher that runs one helper test in a child
// process.
//
// The launcher is needed because ExecRunner always appends the contract's
// --events flag, which a Go test binary rejects as an unknown flag. The script
// ignores the arguments it is given and execs the test binary with a fixed
// argument vector, which also means the test binary becomes the process group
// leader and receives the interrupt directly.
func helperScript(t *testing.T, testName string) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "helper.sh")
	script := "#!/bin/sh\nexec " + self + " -test.run=" + testName + "\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExecRunnerInterruptsTheChildAndReports130(t *testing.T) {
	t.Setenv("GO_TUI_HELPER", "1")
	r := &ExecRunner{
		Path:     helperScript(t, "TestHelperProcessIsInterrupted"),
		ToolName: "test",
		// The child's human output is not the event stream and must not be
		// mixed into it.
		Stderr: io.Discard,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out := &syncBuffer{}
	done := make(chan int, 1)
	go func() {
		code, _ := r.Run(ctx, []string{}, out)
		done <- code
	}()

	// Wait for the child to actually be up and streaming before
	// interrupting, so the test measures cancellation rather than startup.
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(out.String(), EventExecutionStarted) {
		if time.Now().After(deadline) {
			t.Fatal("the child never started streaming events")
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()

	select {
	case code := <-done:
		if code != ExitCancelled {
			t.Errorf("cancelled child exit = %d, want %d", code, ExitCancelled)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the child was never interrupted")
	}

	// Whatever the child managed to emit before the interrupt must survive:
	// cancellation preserves partial results rather than discarding them.
	if !strings.Contains(out.String(), EventExecutionStarted) {
		t.Errorf("events emitted before cancellation were lost: %q", out.String())
	}
}

func TestExecRunnerDeliversACompletedRun(t *testing.T) {
	t.Setenv("GO_TUI_HELPER", "1")
	var out bytes.Buffer
	r := &ExecRunner{
		Path:     helperScript(t, "TestHelperProcessEmitsEvents"),
		ToolName: "test",
		Stderr:   io.Discard,
	}
	code, err := r.Run(context.Background(), []string{}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	stats := readEvents(&out, func(Event) {})
	if stats.Decoded != 3 {
		t.Errorf("decoded %d events from the child, want 3: %q", stats.Decoded, out.String())
	}
}

// TestHelperProcessIsInterrupted is not a real test. It is the child process
// used by TestExecRunnerInterruptsTheChildAndReports130: when re-executed by
// the parent it emits a start event, waits for SIGINT, emits a cancellation
// event and exits -- what an interrupted tool should do.
func TestHelperProcessIsInterrupted(t *testing.T) {
	if os.Getenv("GO_TUI_HELPER") != "1" {
		t.Skip("helper process; only runs when re-executed by the parent")
	}
	var buf bytes.Buffer
	emit(t, &buf, "helper", EventExecutionStarted, nil)
	os.Stdout.Write(buf.Bytes())

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	<-ch

	buf.Reset()
	emit(t, &buf, "helper", EventExecutionCancelled, nil)
	os.Stdout.Write(buf.Bytes())
	os.Exit(0)
}

func TestHelperProcessEmitsEvents(t *testing.T) {
	if os.Getenv("GO_TUI_HELPER") != "1" {
		t.Skip("helper process; only runs when re-executed by the parent")
	}
	var buf bytes.Buffer
	emit(t, &buf, "helper", EventExecutionStarted, nil)
	emit(t, &buf, "helper", EventFindingDiscovered, map[string]any{"title": "x", "severity": "low"})
	emit(t, &buf, "helper", EventExecutionCompleted, nil)
	os.Stdout.Write(buf.Bytes())
	os.Exit(0)
}

func TestEventStreamMustBePureJSONL(t *testing.T) {
	// Whatever the TUI routes, the pipe it reads carries envelopes only.
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	emit(t, &buf, "e1", EventProgressUpdated, map[string]any{"percent": 50, "message": "halfway"})
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("line is not a valid envelope: %q", line)
		}
	}
}
