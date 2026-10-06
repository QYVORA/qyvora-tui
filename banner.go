package tui

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
)

// Variant is one rung of the banner ladder. A tool name is rendered at the
// richest variant that fits the width it was given, so the identity is as large
// as the terminal allows and is never truncated into nonsense.
type Variant int

const (
	// VariantAuto lets RenderBanner choose.
	VariantAuto Variant = iota
	// VariantFull is the five-row pixel wordmark: one glyph per letter.
	VariantFull
	// VariantCompact is the three-row wordmark: half the rows, same width.
	VariantCompact
	// VariantStacked splits the name across rows for a narrow terminal.
	VariantStacked
	// VariantMarquee is a single bordered row carrying the name.
	VariantMarquee
	// VariantNone is no art at all. The name has no legible rendering at the
	// available width, so the header carries identity on its own instead.
	VariantNone
)

func (v Variant) String() string {
	switch v {
	case VariantFull:
		return "full"
	case VariantCompact:
		return "compact"
	case VariantStacked:
		return "stacked"
	case VariantMarquee:
		return "marquee"
	case VariantNone:
		return "none"
	default:
		return "auto"
	}
}

// Banner is the identity a tool hands the interface. It is a description, not
// artwork: the wordmark is generated from Tool in the standard block font, so a
// tool gets a wordmark without shipping one, and the same name always produces
// the same picture.
type Banner struct {
	// Tool is the product name, with or without a QYVORA prefix. It is the word
	// the banner draws.
	Tool string
	// Tagline is an optional line set under the wordmark.
	Tagline string
	// Version is optional and is normally drawn by the header rather than the
	// banner; it is kept here so a caller can pass everything it knows in one
	// value.
	Version string
	// Glyphs names the box-drawing set used for framed variants. Empty means
	// rounded.
	Glyphs string
	// Variant pins the rung instead of letting RenderBanner choose. Tests use
	// this; the interface leaves it Auto.
	Variant Variant
	// Art overrides the generated wordmark wholesale. A tool that wants a
	// hand-drawn mark supplies it here and still gets the same fit rules.
	Art []string
}

// RenderedBanner is a wordmark that has already been chosen and measured.
type RenderedBanner struct {
	// ASCII is the art, one string per row, with no colour applied.
	ASCII []string
	// Width and Height are the measured extent of ASCII.
	Width  int
	Height int
	// Variant is the rung that produced this art.
	Variant Variant
	// Dropped is true when the name had no legible rendering at the available
	// width. A caller must not draw a partial banner in that case; the header
	// already carries the tool name.
	Dropped bool
}

// IsDropped reports whether the banner carries no art.
func (r RenderedBanner) IsDropped() bool { return r.Dropped || len(r.ASCII) == 0 }

// ToolBanner builds a Banner with the house defaults for a QYVORA tool.
func ToolBanner(tool, tagline string) Banner {
	return Banner{Tool: tool, Tagline: titleCase(tool) + " " + tagline}
}

// fullHeight is the row count of the five-row wordmark.
const fullHeight = 5

// compactHeight is the row count of the three-row wordmark.
const compactHeight = 3

// stackedGlyphs is the widest name fragment the stacked variant draws before it
// wraps to another row. Six glyphs is where a fragment stops reading as part of
// the name and starts reading as a word.
const stackedGlyphs = 6

// maxFullWidth is the widest the full wordmark is ever allowed to be. The
// longest QYVORA name, AMANIRENAS, is fifty-nine columns at six to a letter, so
// this is one column of headroom rather than a width the art ever reaches.
const maxFullWidth = 60

// maxBannerRows caps the stacked variant, which is the only rung whose height
// grows with the number of name fragments. A banner is furniture: past a dozen
// rows it is competing with the transcript for the terminal.
const maxBannerRows = 12

// RenderBanner picks the richest variant of spec.Tool that fits width and, when
// height is a positive constraint, fits that too.
//
// A width of zero or less is treated as unconstrained, because a caller that
// has not measured yet should get the full wordmark and clamp it itself rather
// than be handed nothing.
func RenderBanner(spec Banner, width, height int) RenderedBanner {
	if len(spec.Art) > 0 {
		return measuredExplicit(spec.Art, width, height, spec.Variant)
	}
	name := normaliseName(spec.Tool)
	if name == "" {
		return RenderedBanner{Variant: VariantNone, Dropped: true}
	}
	for _, v := range []Variant{VariantFull, VariantCompact, VariantStacked, VariantMarquee} {
		art := renderVariant(name, spec, v)
		if art.IsDropped() {
			continue
		}
		if !v.fits(art, width, height) {
			continue
		}
		return art
	}
	// Nothing fits. Returning empty art rather than a truncated name is the whole
	// point of the ladder: a half-drawn wordmark is worse than no wordmark,
	// because the operator has to read past the damage to find the tool name.
	return RenderedBanner{Variant: VariantNone, Dropped: true}
}

// fits reports whether a rendered banner fits the offered space. A non-positive
// dimension means the caller is not constraining that axis.
func (v Variant) fits(r RenderedBanner, width, height int) bool {
	if width > 0 && r.Width > width {
		return false
	}
	if height > 0 && r.Height > height {
		return false
	}
	return r.Height <= maxBannerRows
}

func renderVariant(name string, spec Banner, v Variant) RenderedBanner {
	var ascii []string
	switch v {
	case VariantFull:
		ascii = wordmark(name, fontFull)
	case VariantCompact:
		ascii = wordmark(name, fontCompact)
	case VariantStacked:
		ascii = stackedWordmark(name, fontCompact, stackedGlyphs)
	case VariantMarquee:
		ascii = []string{glyphRightHalf + " " + name + " " + glyphLeftHalf}
	default:
		return RenderedBanner{Variant: VariantNone, Dropped: true}
	}
	ascii = appendASCII(ascii, taglineLine(spec))
	return measureASCII(ascii, v)
}

// appendASCII adds the tagline as a final row when the art has no explicit
// override and the caller supplied one. The tagline is dropped rather than
// clipped if it is wider than the widest art row, because a clipped tagline
// reads as a rendering fault.
func appendASCII(art []string, tagline string) []string {
	if tagline == "" {
		return art
	}
	w := 0
	for _, l := range art {
		if n := lipgloss.Width(l); n > w {
			w = n
		}
	}
	if lipgloss.Width(tagline) > w {
		return art
	}
	return append(append([]string(nil), art...), tagline)
}

func measuredExplicit(art []string, width, height int, v Variant) RenderedBanner {
	if v == VariantAuto {
		v = VariantFull
	}
	rows := append([]string(nil), art...)
	if height > 0 && len(rows) > height {
		rows = rows[:height]
	}
	for _, l := range rows {
		if width > 0 && lipgloss.Width(l) > width {
			return RenderedBanner{Variant: VariantNone, Dropped: true}
		}
	}
	return measureASCII(rows, v)
}

func measureASCII(art []string, v Variant) RenderedBanner {
	w := 0
	for _, l := range art {
		if n := lipgloss.Width(l); n > w {
			w = n
		}
	}
	return RenderedBanner{ASCII: art, Width: w, Height: len(art), Variant: v}
}

// stackedWordmark splits a name into fragments and draws one under the next, so
// a name too wide for the terminal is made narrow rather than wrapped.
func stackedWordmark(name string, font pixelFont, maxGlyphs int) []string {
	chunks := splitName(name, maxGlyphs)
	if len(chunks) <= 1 {
		// A name that already fits one fragment gets the plain wordmark, so the
		// stacked variant is never the same picture twice at different widths.
		return nil
	}
	var out []string
	for _, c := range chunks {
		out = append(out, wordmark(c, font)...)
	}
	return out
}

// splitName divides a name into fragments of at most maxGlyphs characters,
// balanced so the rows come out as even as possible. Balancing rather than
// filling matters: "AMANI/RENAS" reads as one word broken in half, while
// "AMAN/RENAS" does not.
func splitName(name string, maxGlyphs int) []string {
	if maxGlyphs < 1 {
		maxGlyphs = 1
	}
	if len(name) <= maxGlyphs {
		return []string{name}
	}
	n := (len(name) + maxGlyphs - 1) / maxGlyphs
	base, extra := len(name)/n, len(name)%n
	var out []string
	for i := 0; i < n; i++ {
		size := base
		if i < extra {
			size++
		}
		out = append(out, name[:size])
		name = name[size:]
	}
	return out
}

// wordmark draws one row per font row, joining glyphs with a single column of
// space. The trailing gap is omitted so the art's width is 6n-1 rather than 6n,
// which is what keeps a ten-letter name inside sixty columns.
func wordmark(name string, font pixelFont) []string {
	rows := make([]string, 0, font.height)
	for r := 0; r < font.height; r++ {
		var b strings.Builder
		for i, ch := range name {
			if i > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(font.glyph(ch)[r])
		}
		rows = append(rows, b.String())
	}
	return rows
}

// normaliseName reduces a tool name to what the wordmark font can draw: upper
// case letters, digits, and hyphens. Anything else becomes a space so an
// unexpected name degrades to a slightly narrower wordmark instead of a row of
// blank glyphs.
func normaliseName(raw string) string {
	raw = strings.TrimSpace(raw)
	if i := strings.LastIndex(raw, "/"); i >= 0 {
		raw = raw[i+1:]
	}
	// A repository or binary name arrives as "qyvora-amanirenas" or
	// "qyvora/amanirenas"; only the product segment is drawn. Only a leading
	// QYVORA segment is stripped, so a name that genuinely contains a hyphen
	// keeps it rather than losing its first syllable to a heuristic.
	if len(raw) > 6 {
		if head, tail := raw[:6], raw[6:]; strings.EqualFold(head, "qyvora") && (tail[0] == '-' || tail[0] == '/' || tail[0] == '_') {
			raw = tail[1:]
		}
	}
	var b strings.Builder
	for _, r := range strings.ToUpper(raw) {
		switch {
		case unicode.IsLetter(r) && r < unicode.MaxASCII:
			b.WriteRune(r)
		case unicode.IsDigit(r) && r < unicode.MaxASCII:
			b.WriteRune(r)
		default:
			b.WriteByte(' ')
		}
	}
	return strings.TrimSpace(strings.Join(strings.Fields(b.String()), ""))
}

// taglineLine is the caption under a wordmark. It is empty when the caller gave
// nothing, because an empty row under art is a hole in the layout.
func taglineLine(spec Banner) string {
	return strings.TrimSpace(spec.Tagline)
}

// titleCase upper-cases the first rune and lower-cases the rest, which is what a
// product name wants. It is not used for a sentence, only for a name.
func titleCase(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	for i := 1; i < len(r); i++ {
		r[i] = unicode.ToLower(r[i])
	}
	return string(r)
}
