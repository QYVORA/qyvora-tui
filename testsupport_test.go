package tui

import (
	"bytes"
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// windowSizeMsg adapts a width and height into the resize message bubbletea
// delivers, so tests can exercise the resize path without a real terminal.
func windowSizeMsg(w, h int) tea.WindowSizeMsg { return tea.WindowSizeMsg{Width: w, Height: h} }

var _ = os.Stdout
var _ = strings.TrimSpace

// resize applies a terminal size to a model and returns the result.
//
// Update takes the model by value, as bubbletea requires, so the resized
// state has to come from its return value. Assigning to the original would
// silently test the pre-resize layout.
func resize(m model, w, h int) model {
	out, _ := m.Update(windowSizeMsg(w, h))
	return out.(model)
}

// syncBuffer is a bytes.Buffer that is safe to read while a runner goroutine
// is still writing to it.
//
// The runner drains the child's event stream on its own goroutine, so a test
// that polls the output to decide when the child is up would otherwise race
// with that drain.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// addBlock appends a block and pushes it into the viewport, mirroring what the
// model does when a command starts.
func addBlock(m model, b *block) model {
	m.blocks = append(m.blocks, b)
	m.activeID = b.id
	m.refreshViewport()
	return m
}

// refresh re-renders the transcript into the viewport.
func refresh(m model) model {
	m.refreshViewport()
	return m
}

// nowish returns a timestamp for tests that need a plausible duration.
func nowish() time.Time { return time.Now() }

// testPalette is a tool palette that differs from the base in exactly one slot,
// used to prove a partial theme is legal and inherits the rest.
var testPalette = Palette{Accent: "#7B61FF"}

// normalize is the shared helper for capability-normalisation tests.
func normalize(t *testing.T, tool, json string) *Capabilities {
	t.Helper()
	c, err := NormalizeCapabilities(tool, []byte(json))
	if err != nil {
		t.Fatalf("normalising %s: %v", tool, err)
	}
	return c
}

// testCommands is a command tree shaped like a real one: top-level words with
// subcommands beneath them. A form resolves a capability's ID against this, so a
// runner with no commands would refuse every submission, which is right in
// production and useless in a test.
var testCommands = []Command{
	{Name: "analyze", Short: "Analyze the latest session for security findings"},
	{Name: "assess", Short: "Run the full wireless assessment pipeline"},
	{Name: "discover", Short: "Discover wireless interfaces"},
	{Name: "scan", Short: "Scan for wireless networks"},
	{Name: "session", Short: "Session commands", Subs: []string{"list", "show"}},
}

// modelFor builds a model at a size with a runner that succeeds instantly.
func modelFor(t *testing.T, w, h int, caps *Capabilities) model {
	t.Helper()
	m := newModel(Config{
		Title:   "QYVORA / PROBE",
		Version: "0.1.0",
		Runner: &InProcessRunner{
			ToolName: "probe",
			Execute:  func(context.Context, []string) int { return 0 },
			Meta:     testCommands,
		},
		Capabilities: caps,
	}, newTheme(false, nil))
	// model is a value type, so the update has to be taken back: dropping the
	// returned model leaves the model at its constructed size and every
	// width-dependent assertion silently tests 80 columns.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return updated.(model)
}
