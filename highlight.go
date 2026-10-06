package tui

import (
	"regexp"
	"strings"
)

// Tool output is the one part of the interface the shared layer did not
// produce, so highlighting it has to be conservative: every token the
// highlighter recognises is a token it has decided to mean something, and a
// wrong guess is worse than no colour. The classes below are limited to things
// a scan, a lint or a build prints and means the same way every time.
//
// Highlighting must also be invisible in the text. The tool printed its own
// capitalisation, spacing and wording, and the highlighter is colouring words
// out of that output, not editing it: a tool printing "high" is high, and a
// transcript that says "HIGH" because the interface felt like it is a transcript
// that no longer matches the tool. Stripping the styling off a highlighted line
// has to return the tool's bytes exactly.

// highlightToken is one recognisable class of token in a tool's own output.
type highlightToken struct {
	pattern *regexp.Regexp
	style   func(Theme) lipglossStyle
}

// highlightTokens are the classes worth picking out of a tool's own output.
//
// Severities are matched case-insensitively because tools disagree about
// capitalisation, and a run that prints "warning" must not be styled differently
// from one that prints "WARNING". The word is styled in whatever case it
// arrived, which is the only way to keep the round trip exact.
var highlightTokens = []highlightToken{
	// Severities, which are the loudest signal a linter produces.
	{regexp.MustCompile(`(?i)\b(?:critical|fatal)\b`), func(t Theme) lipglossStyle { return t.Critical }},
	{regexp.MustCompile(`(?i)\b(?:error|err)\b`), func(t Theme) lipglossStyle { return t.Critical }},
	{regexp.MustCompile(`(?i)\bhigh\b`), func(t Theme) lipglossStyle { return t.High }},
	{regexp.MustCompile(`(?i)\b(?:medium|moderate|warn|warning)\b`), func(t Theme) lipglossStyle { return t.Medium }},
	{regexp.MustCompile(`(?i)\b(?:low|note)\b`), func(t Theme) lipglossStyle { return t.Low }},
	{regexp.MustCompile(`(?i)\b(?:info|debug|trace)\b`), func(t Theme) lipglossStyle { return t.Info }},

	// Verdict words. A run's outcome is the first thing an operator looks for,
	// and it is the word most likely to be buried in the middle of a line.
	{regexp.MustCompile(`(?i)\b(?:passed|passing)\b`), func(t Theme) lipglossStyle { return t.Success }},
	{regexp.MustCompile(`(?i)\bok\b`), func(t Theme) lipglossStyle { return t.Success }},
	{regexp.MustCompile(`(?i)\b(?:degraded|partial|skipped)\b`), func(t Theme) lipglossStyle { return t.Medium }},
	{regexp.MustCompile(`(?i)\b(?:failed|failing|failure|timeout|timed out)\b`), func(t Theme) lipglossStyle { return t.Failed }},
	{regexp.MustCompile(`(?i)\b(?:cancelled|canceled|aborted)\b`), func(t Theme) lipglossStyle { return t.Cancelled }},

	// Measurements, so a scan's numbers can be found without reading the
	// sentence around them.
	{regexp.MustCompile(`\b\d+(?:\.\d+)?(?:%|ms|s|kb|mb|gb|kib|mib|gib|b|px)\b`), func(t Theme) lipglossStyle { return t.Value }},

	// Paths and addresses, which are what an operator copies out of the
	// transcript. Cyan is the hue the theme reserves for "look here".
	//
	// Both ends matter. Absolute paths admit the root slash, so that
	// "/var/log/app.log" arrives whole rather than with a bare "/" left in front
	// of it, and the trailing component is written as dots-and-dashes followed by
	// a word-ending character so a filename's extension belongs to the path. A
	// pattern that ended on a bare \b matched "src/error" out of "src/error.c"
	// and left the extension to be styled as something else.
	//
	// Absolute and relative are separate patterns rather than one optional
	// slash, because an optional slash lets the engine satisfy the match from
	// the middle of a relative path: on "src/error.c" it starts at the slash and
	// returns "/error.c", which is how the leading directory gets lost. Split, the
	// relative pattern has to start at "src" because that is the only place its
	// first component can begin.
	{regexp.MustCompile(`(?:\.{1,2})?/(?:[\w.-]+/)*[\w.-]*[\w-]+`), func(t Theme) lipglossStyle { return t.Cyan }},
	{regexp.MustCompile(`(?:\.{1,2}/)?(?:[\w.-]+/)+[\w.-]*[\w-]+`), func(t Theme) lipglossStyle { return t.Cyan }},

	// Hosts on their own, and addresses inside them.
	{regexp.MustCompile(`\b[\w-]+(?:\.[\w-]+)*\.(?:com|net|org|io|dev|local)\b`), func(t Theme) lipglossStyle { return t.Cyan }},
	{regexp.MustCompile(`\b[\w-]+:[\d.]+\b`), func(t Theme) lipglossStyle { return t.Cyan }},
}

// findHighlightToken returns the longest class match starting at or after from,
// preferring the earliest start and then the longest span.
//
// Longest-wins at a position is what keeps a severity word inside a path from
// being painted: at the "s" of "src/error.c" the path class matches all of it,
// so the scan consumes the whole path and never reaches the "error" inside. A
// class-by-class pass cannot do this, because the severity class is tried first
// and claims the substring before the path class is ever consulted.
func findHighlightToken(s string, from int, t Theme) (int, int, lipglossStyle, bool) {
	bestStart, bestEnd := -1, -1
	var bestStyle lipglossStyle
	for _, tok := range highlightTokens {
		loc := tok.pattern.FindStringIndex(s[from:])
		if loc == nil {
			continue
		}
		start, end := from+loc[0], from+loc[1]
		switch {
		case bestStart < 0 || start < bestStart:
			bestStart, bestEnd, bestStyle = start, end, tok.style(t)
		case start == bestStart && end > bestEnd:
			bestEnd, bestStyle = end, tok.style(t)
		}
	}
	return bestStart, bestEnd, bestStyle, bestStart >= 0
}

// highlightLine styles the tokens it recognises in one already-wrapped line of
// tool output.
//
// It runs on the wrapped line, never on the line before wrapping. That is the
// whole ordering constraint: a wrapper that measures a string containing escape
// sequences counts them as characters and breaks in the middle of one, so a
// coloured word arrives as a smear. Wrapping first means the width arithmetic
// only ever sees plain text.
//
// Every byte that is not part of a recognised token is copied through untouched,
// including whitespace. The spaces in a table are the table, so the highlighter
// preserves runs of them exactly as wrapText left them.
//
// Unstyled text keeps the Output style rather than becoming default-coloured, so
// the tool's output looks the way it always has and only the picked-out tokens
// differ from it.
func highlightLine(s string, t Theme) string {
	if s == "" {
		return ""
	}
	// Colour off, or a line that already carries escape sequences. The transcript
	// has no codes of its own by the time it gets here, but a runner can inject
	// one, and re-styling around a sequence the highlighter does not understand
	// is how that sequence ends up half-consumed.
	if !t.Color || strings.ContainsRune(s, 0x1b) {
		return t.Output.Render(s)
	}

	segs := highlightSegments(s, t)
	var b strings.Builder
	b.Grow(len(s) + 16)
	for _, seg := range segs {
		if seg.text == "" {
			// Rendering an empty string still costs a set and a reset, and an
			// empty segment carries no information for the reader to miss.
			continue
		}
		b.WriteString(seg.style.Render(seg.text))
	}
	if b.Len() == 0 {
		return t.Output.Render(s)
	}
	return b.String()
}

// segment is one run of text and the style it is drawn in.
type segment struct {
	text  string
	style lipglossStyle
}

// highlightSegments splits a line into alternating unstyled and styled runs.
//
// It is separate from the rendering so the tokenisation can be tested on its own,
// and so the count of what was claimed does not depend on reading escape
// sequences back out of a rendered string.
func highlightSegments(s string, t Theme) []segment {
	var segs []segment
	last := 0
	for {
		start, end, style, ok := findHighlightToken(s, last, t)
		if !ok {
			break
		}
		if start > last {
			segs = append(segs, segment{s[last:start], t.Output})
		}
		segs = append(segs, segment{s[start:end], style})
		last = end
	}
	if last == 0 {
		return []segment{{s, t.Output}}
	}
	if last < len(s) {
		segs = append(segs, segment{s[last:], t.Output})
	}
	return segs
}
