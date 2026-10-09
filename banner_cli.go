package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ThinBanner builds the single-line banner a tool prints in place of its
// wordmark when the wordmark would not fit the terminal. It carries the name
// between two rules and the tagline after, so the identity and the descriptor
// survive a narrow window that cannot draw the art.
func ThinBanner(name, tagline string) string {
	name = strings.ToUpper(strings.TrimSpace(name))
	if name == "" {
		name = "QYVORA"
	}
	line := "── " + name + " ──"
	if tagline = strings.TrimSpace(tagline); tagline != "" {
		line += " " + tagline
	}
	return line
}

// RenderCLIArt is the width-aware rule every tool's CLI banner follows: when
// the art fits the terminal it is returned with its common leading column
// removed (so every tool's mark starts at the same place), and when it does
// not fit, a thin banner that cannot wrap is returned instead. An art block is
// never drawn cut or shifted mid-glyph.
//
// A non-positive width is treated as unconstrained: a caller that could not
// measure the terminal (a pipe, a file, a log) gets the full art, which is the
// form that belongs in an archive.
func RenderCLIArt(art []string, width int, name, tagline string) []string {
	art = normaliseCLIArt(art)
	if width <= 0 {
		return art
	}
	if maxCLIRowWidth(art) <= width {
		return art
	}
	return []string{fitThinBanner(name, tagline, width)}
}

// normaliseCLIArt drops the rows that carry no glyph and removes the leading
// column shared by every remaining row. The mark reads the same; it just
// starts at the same place on every tool instead of at a per-tool offset.
func normaliseCLIArt(art []string) []string {
	var rows []string
	min := -1
	for _, l := range art {
		if strings.TrimSpace(l) == "" {
			continue
		}
		l = strings.TrimRight(l, " ")
		lead := leadingSpaces(l)
		if min < 0 || lead < min {
			min = lead
		}
		rows = append(rows, l)
	}
	if min <= 0 {
		return rows
	}
	for i := range rows {
		if len(rows[i]) >= min {
			rows[i] = rows[i][min:]
		}
	}
	return rows
}

func leadingSpaces(s string) int {
	n := 0
	for _, r := range s {
		if r != ' ' {
			break
		}
		n++
	}
	return n
}

func maxCLIRowWidth(art []string) int {
	w := 0
	for _, l := range art {
		if n := lipgloss.Width(l); n > w {
			w = n
		}
	}
	return w
}

// fitThinBanner trims the thin banner to a terminal: tagline first, then the
// rules, and finally the bare name, which is the most a terminal can ever need.
func fitThinBanner(name, tagline string, width int) string {
	name = strings.ToUpper(strings.TrimSpace(name))
	if name == "" {
		name = "QYVORA"
	}
	candidates := []string{
		ThinBanner(name, tagline),
		ThinBanner(name, ""),
		name,
	}
	for _, c := range candidates {
		if lipgloss.Width(c) <= width {
			return c
		}
	}
	return name
}
