package tui

// historyLimit caps how many commands are remembered. A long session should
// not grow without bound; the oldest entries fall off the top.
const historyLimit = 500

// History is the command input history, navigated with the up and down arrows.
//
// It remembers the in-progress line that was being edited when navigation
// began, so pressing down past the newest entry restores what the user was
// typing rather than clearing the input.
type History struct {
	entries []string
	pos     int
	// draft is the line the user was editing before they started navigating.
	draft string
	saved bool
}

// NewHistory returns an empty history.
func NewHistory() *History { return &History{pos: -1} }

// Add records a command, skipping blanks and immediate repeats.
func (h *History) Add(line string) {
	if line == "" {
		return
	}
	if n := len(h.entries); n > 0 && h.entries[n-1] == line {
		h.reset()
		return
	}
	h.entries = append(h.entries, line)
	if len(h.entries) > historyLimit {
		h.entries = h.entries[len(h.entries)-historyLimit:]
	}
	h.reset()
}

func (h *History) reset() {
	h.pos = -1
	h.draft = ""
	h.saved = false
}

// Prev moves one entry back in history. The second return value is false when
// there is nothing older, which leaves the input untouched.
func (h *History) Prev(current string) (string, bool) {
	if len(h.entries) == 0 {
		return "", false
	}
	if !h.saved {
		h.draft = current
		h.saved = true
	}
	if h.pos == -1 {
		h.pos = len(h.entries) - 1
	} else if h.pos > 0 {
		h.pos--
	}
	// At the oldest entry, stay put rather than reporting nothing: holding
	// up should keep showing the oldest command, as a shell history does.
	return h.entries[h.pos], true
}

// Next moves one entry forward, returning the saved draft once past the newest
// entry.
func (h *History) Next(_ string) (string, bool) {
	if len(h.entries) == 0 || h.pos == -1 {
		return "", false
	}
	if h.pos < len(h.entries)-1 {
		h.pos++
		return h.entries[h.pos], true
	}
	// Past the end: restore what was being typed. The draft is captured before
	// the reset, which clears it.
	draft := h.draft
	h.reset()
	return draft, true
}

// Entries returns the recorded history, oldest first.
func (h *History) Entries() []string {
	out := make([]string, len(h.entries))
	copy(out, h.entries)
	return out
}
