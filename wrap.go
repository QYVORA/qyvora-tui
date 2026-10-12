package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func wrapText(s string, width int) []string {
	s = strings.ReplaceAll(strings.TrimRight(s, "\r"), "\t", "    ")
	if width < 8 {
		return []string{s}
	}
	if lipgloss.Width(s) <= width {
		return []string{s}
	}

	// Width is display width, not rune count: a line of CJK text or an emoji is
	// wider than its length, and measuring runes lets it overflow the column.
	var b lineBuilder
	b.limit = width
	b.runes = make([]rune, 0, width)
	out := make([]string, 0, 4)
	// A line of nothing but spaces is not a line. It appears when the
	// indentation is wider than the width, and drawing it would push everything
	// else down the transcript for no information.
	flush := func() {
		line := strings.TrimRight(b.String(), " ")
		b.reset()
		if strings.TrimSpace(line) == "" {
			return
		}
		out = append(out, line)
	}

	// Indentation belongs to the line it starts on, and is kept for exactly that
	// long. Indentation past the width is dropped rather than wrapping into a
	// line of nothing but spaces.
	runes := []rune(s)
	i := 0
	for i < len(runes) && runes[i] == ' ' && b.fits(1) {
		b.add(runes[i])
		i++
	}

	// pending counts the spaces seen since the last word, so a word can be placed
	// together with the gap that introduces it -- which is what keeps two columns
	// two columns.
	pending := 0
	for i < len(runes) {
		if runes[i] == ' ' {
			pending++
			i++
			continue
		}
		start := i
		for i < len(runes) && runes[i] != ' ' {
			i++
		}
		word := runes[start:i]
		ww := lipgloss.Width(string(word))

		switch {
		case b.empty():
			// The first word on a line. Any spaces left over are interior
			// alignment from the original and are kept as far as they fit.
			b.addSpaces(pending)
			pending = 0
			b.addWord(&out, flush, word)
		case b.fits(pending + ww):
			b.addSpaces(pending)
			pending = 0
			b.addWord(&out, flush, word)
		default:
			// The word does not fit after the gap. Break here, and the gap is the
			// break: it goes rather than starting the next line with it.
			pending = 0
			flush()
			b.addWord(&out, flush, word)
		}
	}
	flush()
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

// lineBuilder accumulates one wrapped line, tracking its display width.
type lineBuilder struct {
	runes []rune
	width int
	limit int
}

func (b *lineBuilder) empty() bool { return len(b.runes) == 0 }

func (b *lineBuilder) fits(w int) bool { return b.width+w <= b.limit }

func (b *lineBuilder) add(r rune) {
	b.runes = append(b.runes, r)
	b.width += lipgloss.Width(string(r))
}

func (b *lineBuilder) addSpaces(n int) {
	for i := 0; i < n && b.fits(1); i++ {
		b.add(' ')
	}
}

func (b *lineBuilder) reset() {
	b.runes = b.runes[:0]
	b.width = 0
}

func (b *lineBuilder) String() string { return string(b.runes) }

// addWord appends a word, breaking the line as many times as the word needs.
//
// A word wider than the whole line cannot be made to fit by any amount of
// breaking, so it is split by width. It is never dropped.
func (b *lineBuilder) addWord(out *[]string, flush func(), word []rune) {
	for _, r := range word {
		w := lipgloss.Width(string(r))
		if !b.fits(w) && !b.empty() {
			flush()
		}
		// A single glyph wider than the line, such as a full-width CJK character
		// on an eight-column transcript, goes on anyway: clipping it would lose
		// it, and one overflowing glyph is better than none.
		if !b.fits(w) && w > b.limit {
			b.add(r)
			continue
		}
		b.add(r)
	}
}
