package tui

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
)

func TestInTUISessionDefaultsToFalse(t *testing.T) {
	// The default has to be "not in a session": a standalone run, or a tool that
	// never consults the signal, must print its banner exactly as it does today.
	t.Setenv(TUISessionEnv, "")
	if InTUISession() {
		t.Fatal("an unset session signal reported a TUI session")
	}
	_ = os.Unsetenv(TUISessionEnv)
	if InTUISession() {
		t.Fatal("a missing session signal reported a TUI session")
	}
}

func TestInTUISessionReadsTheSignal(t *testing.T) {
	for _, v := range []string{"1", "true", "TRUE", "yes", "on"} {
		t.Setenv(TUISessionEnv, v)
		if !InTUISession() {
			t.Errorf("%q was not read as a TUI session", v)
		}
	}
	for _, v := range []string{"0", "false", "no", "", "off"} {
		t.Setenv(TUISessionEnv, v)
		if InTUISession() {
			t.Errorf("%q was read as a TUI session", v)
		}
	}
}

func TestEnterTUISessionRestoresThePreviousValue(t *testing.T) {
	// Unset before: it must be unset again, not left as an empty string, or a
	// later LookupEnv would claim the variable exists.
	_ = os.Unsetenv(TUISessionEnv)
	restore := enterTUISession()
	if !InTUISession() {
		t.Fatal("the signal is not set during the session")
	}
	restore()
	if _, had := os.LookupEnv(TUISessionEnv); had {
		t.Error("the signal was left behind after a run that did not start with it")
	}

	// Set before: the old value has to come back, not be cleared.
	t.Setenv(TUISessionEnv, "keepme")
	restore = enterTUISession()
	if !InTUISession() {
		t.Fatal("the signal is not set during the session")
	}
	restore()
	if got := os.Getenv(TUISessionEnv); got != "keepme" {
		t.Errorf("restored value = %q, want %q", got, "keepme")
	}
}

func TestChildEnvCarriesTheSignal(t *testing.T) {
	env := childEnv([]string{"PATH=/bin", "HOME=/root"})
	if !containsEnv(env, TUISessionEnv+"=1") {
		t.Errorf("the child environment is missing the session signal: %v", env)
	}
	if !containsEnv(env, "PATH=/bin") || !containsEnv(env, "HOME=/root") {
		t.Errorf("the child environment dropped inherited variables: %v", env)
	}
	// The base slice must not be mutated in place.
	base := []string{"PATH=/bin"}
	_ = childEnv(base)
	if len(base) != 1 {
		t.Errorf("childEnv appended to its caller's slice: %v", base)
	}
}

func containsEnv(env []string, want string) bool {
	for _, e := range env {
		if e == want {
			return true
		}
	}
	return false
}

func TestInProcessRunnerSetsTheSessionSignalForTheTool(t *testing.T) {
	// The tool runs in this process, so it must see the signal through the
	// environment exactly while Execute runs, and not after.
	_ = os.Unsetenv(TUISessionEnv)
	var seen bool
	r := &InProcessRunner{
		ToolName: "test",
		Execute: func(context.Context, []string) int {
			seen = InTUISession()
			return 0
		},
	}
	if _, err := r.Run(context.Background(), []string{"scan"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !seen {
		t.Error("the tool did not observe the session signal while it ran")
	}
	if InTUISession() {
		t.Error("the signal leaked out of the run")
	}
}

// TestHelperProcessSeesTheSessionSignal is the child half of
// TestExecRunnerPassesTheSignalToTheChild.
func TestHelperProcessSeesTheSessionSignal(t *testing.T) {
	if os.Getenv("GO_TUI_HELPER") != "1" {
		t.Skip("helper process; only runs when re-executed by the parent")
	}
	if !InTUISession() {
		_, _ = os.Stderr.WriteString("NO_SESSION_SIGNAL\n")
		os.Exit(3)
	}
	var buf bytes.Buffer
	emit(t, &buf, "helper", EventExecutionCompleted, nil)
	_, _ = os.Stdout.Write(buf.Bytes())
	os.Exit(0)
}

func TestExecRunnerPassesTheSignalToTheChild(t *testing.T) {
	// A child process cannot see the parent's Setenv, so the runner has to put
	// the signal in the child's own environment. The helper exits non-zero and
	// writes a marker to stderr when the signal is missing.
	t.Setenv("GO_TUI_HELPER", "1")
	var stderr bytes.Buffer
	r := &ExecRunner{
		Path:     helperScript(t, "TestHelperProcessSeesTheSessionSignal"),
		ToolName: "test",
		Stderr:   &stderr,
	}
	code, err := r.Run(context.Background(), []string{}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("the child did not see the session signal (exit %d, stderr %q)", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "NO_SESSION_SIGNAL") {
		t.Errorf("the child reported a missing session signal: %q", stderr.String())
	}
}
