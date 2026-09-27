package tui

import (
	"bytes"
	"os"
	"strings"
	"sync"

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
