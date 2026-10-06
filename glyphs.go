package tui

import "strings"

// Glyphs are collected here so the shape of the interface has exactly one
// definition. A component chooses a glyph by name rather than by typing a
// literal, which is what keeps the drawing consistent between the header, the
// sidebar boxes and the banner, and what makes a glyph set swappable without
// touching a single component.
//
// Every rune here is from the Box Drawing block (U+2500-U+257F) or the Block
// Elements block (U+2580-U+259F), both of which are unambiguous in a monospace
// cell. Nothing in this file depends on an emoji-capable font.
const (
	glyphH        = "─" // ─ light horizontal
	glyphV        = "│" // │ light vertical
	glyphTL       = "╭" // ╭ rounded top-left
	glyphTR       = "╮" // ╮ rounded top-right
	glyphBL       = "╰" // ╰ rounded bottom-left
	glyphBR       = "╯" // ╯ rounded bottom-right
	glyphCornerTL = "┌" // ┌ square top-left
	glyphCornerTR = "┐" // ┐ square top-right
	glyphCornerBL = "└" // └ square bottom-left
	glyphCornerBR = "┘" // ┘ square bottom-right
	glyphDH       = "═" // ═ double horizontal
	glyphDV       = "║" // ║ double vertical
	glyphHeavyH   = "━" // ━ heavy horizontal
	glyphHeavyV   = "┃" // ┃ heavy vertical
	glyphCross    = "┼" // ┼ box cross
	glyphTeeDown  = "┬" // ┬ tee down
	glyphTeeUp    = "┴" // ┴ tee up
	glyphTeeRight = "├" // ├ tee right
	glyphTeeLeft  = "┤" // ┤ tee left

	glyphFull      = "█" // █ full block
	glyphLight     = "░" // ░ light shade
	glyphMedium    = "▒" // ▒ medium shade
	glyphDark      = "▓" // ▓ dark shade
	glyphUpperHalf = "▀" // ▀ upper half block
	glyphLowerHalf = "▄" // ▄ lower half block
	glyphLeftHalf  = "▌" // ▌ left half block
	glyphRightHalf = "▐" // ▐ right half block
	glyphQuarter   = "▖" // ▖ lower one quarter block

	glyphTreeMid  = "├" // ├ tree branch
	glyphTreeEnd  = "└" // └ tree terminator
	glyphTick     = "▏" // ▏ left one eighth block
	glyphDot      = "·" // · middle dot
	glyphEllipsis = "…" // … horizontal ellipsis

	glyphChevron = "❯" // ❯ prompt chevron

	// Severity and status marks. A mark never carries meaning on its own: every
	// one of these is always rendered beside a word, so the shape is a scanning
	// aid rather than the signal itself.
	glyphDotFilled    = "●" // ●
	glyphDiamond      = "◆" // ◆
	glyphSquareSmall  = "▮" // ▮
	glyphSquareTiny   = "▯" // ▯
	glyphCheck        = "✓" // ✓
	glyphStatusCross  = "✕" // ✕
	glyphCircleHollow = "○" // ○
	glyphWarn         = "⚠" // ⚠
)

// boxSet is one complete set of corner and edge glyphs for a drawn frame.
//
// The interface draws a lot of frames: the header, the composer, each region in
// the sidebar, each expanded block. Deriving every corner from one place is what
// stops a frame from coming out with a rounded top and a square bottom.
type boxSet struct {
	TL, TR, BL, BR, H, V string
}

// boxSets are the frame styles a caller may ask for.
var (
	boxStandard = boxSet{glyphCornerTL, glyphCornerTR, glyphCornerBL, glyphCornerBR, glyphH, glyphV}
	boxRounded  = boxSet{glyphTL, glyphTR, glyphBL, glyphBR, glyphH, glyphV}
	boxDouble   = boxSet{"╔", "╗", "╚", "╝", glyphDH, glyphDV}
	boxHeavy    = boxSet{"┏", "┓", "┗", "┛", glyphHeavyH, glyphHeavyV}
)

// boxSetByName resolves a caller-supplied set name, falling back to the rounded
// set. An unknown name is not an error: a tool that asks for a frame style this
// build does not know still gets a coherent frame.
func boxSetByName(name string) boxSet {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "standard", "square":
		return boxStandard
	case "double":
		return boxDouble
	case "heavy":
		return boxHeavy
	default:
		return boxRounded
	}
}

// spark returns one of eight block heights for a value in [0,1], for the
// activity sparkline. Values outside the range are clamped rather than indexed,
// so a bad ratio cannot index off the end of the set.
func spark(ratio float64) string {
	set := [...]string{"▁", "▂", "▃", "▄", "▅", "▆", "▇", glyphFull}
	switch {
	case ratio <= 0:
		return set[0]
	case ratio >= 1:
		return set[len(set)-1]
	}
	return set[int(ratio*float64(len(set)-1))]
}
