package tui

import "strings"

// pixelFont is a block-drawing alphabet: every glyph is the same number of rows,
// and a set pixel is a full block while an unset pixel is a space. Nothing here
// is a font lookup at runtime, so a wordmark is data rather than a dependency.
//
// The cell cost is the whole reason for this. figlet gives a five-column slant
// face that happens to look like the tool name; a fixed five-column pixel face
// gives the same width for every name, which is what lets the header reserve the
// same band of the terminal for a four-letter tool as for a ten-letter one.
type pixelFont struct {
	height int
	glyphs map[rune][]string
}

// glyph returns the rows for one rune, padded to the font's height. A rune the
// font does not draw comes back blank rather than as a placeholder: the name is
// normalised before it reaches here, so a blank means the caller passed
// something unexpected, and a hole is a smaller lie than a substitute letter.
func (f pixelFont) glyph(ch rune) []string {
	if rows, ok := f.glyphs[ch]; ok {
		return rows
	}
	return blankRows(f.height)
}

func blankRows(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = strings.Repeat(" ", glyphCellWidth)
	}
	return out
}

// glyphCellWidth is the width of one glyph in cells, excluding the single column
// of space that separates neighbours.
const glyphCellWidth = 5

// newPixelFont expands a slash-separated glyph table into rows.
//
// The table is written as one string per rune with "/" between rows because a
// five-row glyph is unreadable when each row is its own map entry. "/" is a safe
// separator for the same reason normaliseName strips it: a name that reaches the
// font is letters and digits only.
func newPixelFont(height int, table map[rune]string) pixelFont {
	f := pixelFont{height: height, glyphs: make(map[rune][]string, len(table))}
	for ch, spec := range table {
		rows := strings.Split(spec, "/")
		if len(rows) != height {
			panic("tui: pixel font glyph height mismatch for " + string(ch))
		}
		for _, r := range rows {
			if len([]rune(r)) != glyphCellWidth {
				panic("tui: pixel font glyph width mismatch for " + string(ch))
			}
		}
		f.glyphs[ch] = rows
	}
	return f
}

// fontFull is the five-row face. Its art is 6n-1 columns for an n-letter name,
// so the longest QYVORA name, AMANIRENAS, is fifty-nine columns.
var fontFull = newPixelFont(5, map[rune]string{
	'A': ".###./#...#/#####/#...#/#...#",
	'B': "####./#...#/####./#...#/####.",
	'C': ".###./#...#/#..../#...#/.###.",
	'D': "####./#...#/#...#/#...#/####.",
	'E': "#####/#..../####./#..../#####",
	'F': "#####/#..../####./#..../#....",
	'G': ".###./#...#/#.###/#...#/.###.",
	'H': "#...#/#...#/#####/#...#/#...#",
	'I': "#####/..#../..#../..#../#####",
	'J': "####./...#./...#./#..#./.##..",
	'K': "#...#/#..#./###../#..#./#...#",
	'L': "#..../#..../#..../#..../#####",
	'M': "#...#/##.##/#.#.#/#...#/#...#",
	'N': "#...#/##..#/#.#.#/#..##/#...#",
	'O': ".###./#...#/#...#/#...#/.###.",
	'P': "####./#...#/####./#..../#....",
	'Q': ".###./#...#/#...#/#.#.#/.##.#",
	'R': "####./#...#/####./#..#./#...#",
	'S': ".####/#..../.###./....#/####.",
	'T': "#####/..#../..#../..#../..#..",
	'U': "#...#/#...#/#...#/#...#/.###.",
	'V': "#...#/#...#/#...#/.#.#./..#..",
	'W': "#...#/#...#/#.#.#/##.##/#...#",
	'X': "#...#/.#.#./..#../.#.#./#...#",
	'Y': "#...#/.#.#./..#../..#../..#..",
	'Z': "#####/...#./..#../.#.../#####",
	'0': ".###./#...#/#.#.#/#...#/.###.",
	'1': "..#../.##../..#../..#../.###.",
	'2': ".###./#...#/..##./.#.../#####",
	'3': "####./...#./.###./...#./####.",
	'4': "#...#/#...#/#####/....#/....#",
	'5': "#####/#..../####./....#/####.",
	'6': ".###./#..../####./#...#/.###.",
	'7': "#####/....#/...#./..#../..#..",
	'8': ".###./#...#/.###./#...#/.###.",
	'9': ".###./#...#/.####/....#/.###.",
})

// fontCompact is the three-row face. It is the same width as the full face and
// half the height, so it is chosen when rows are scarce rather than when
// columns are.
var fontCompact = newPixelFont(3, map[rune]string{
	'A': ".###./#...#/#####",
	'B': "####./#...#/####.",
	'C': ".####/#..../.####",
	'D': "####./#...#/####.",
	'E': "#####/#..../#####",
	'F': "#####/#..../####.",
	'G': ".####/#...#/.###.",
	'H': "#...#/#...#/#####",
	'I': "#####/..#../#####",
	'J': "####./...#./###..",
	'K': "#..#./###../#..#.",
	'L': "#..../#..../#####",
	'M': "#...#/#####/#...#",
	'N': "#...#/##..#/#..##",
	'O': ".###./#...#/.###.",
	'P': "####./#...#/####.",
	'Q': ".###./#.#.#/.##.#",
	'R': "####./#...#/#..#.",
	'S': ".####/#..../####.",
	'T': "#####/..#../..#..",
	'U': "#...#/#...#/.###.",
	'V': "#...#/#...#/..#..",
	'W': "#...#/#.#.#/.#.#.",
	'X': "#...#/.#.#./#...#",
	'Y': "#...#/.#.#./..#..",
	'Z': "#####/...#./.###.",
	'0': ".###./#.#.#/.###.",
	'1': "..#../..#../.###.",
	'2': ".###./...#./####.",
	'3': "####./..##./####.",
	'4': "#...#/#####/....#",
	'5': "####./#..../####.",
	'6': ".###./#..../.###.",
	'7': "#####/...#./..#..",
	'8': ".###./#...#/.###.",
	'9': ".###./...#./###..",
})
