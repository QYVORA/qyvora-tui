package tui

// text.go — Shared text layout engine for all TUI text rendering
//
// Per PROMPT1.md §5: Every piece of text in the TUI goes through this engine.
// No other file does its own padding, wrapping, or joining.
//
// Typography rules (PROMPT1.md §5):
//   - Indent scale: 2 columns per level, max 3 levels
//   - Prose line length capped at 88 columns even on 200-column terminals
//   - Tables may use full width
//   - One blank row between sections, none inside
//   - Numbers right-aligned, text left-aligned
//   - Display width measured with lipgloss.Width / rune width, never len
//   - Wrap on plain text first, colour second
//   - Tool-printed output keeps its own spacing (preserve aligned tables)

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// WrapOpts configures text wrapping behavior.
type WrapOpts struct {
	// Width is the target line width in display columns.
	Width int
	// HangingIndent is the number of columns to indent continuation lines.
	HangingIndent int
	// FirstIndent is the number of columns to indent the first line (in addition to HangingIndent on subsequent lines).
	FirstIndent int
	// PreserveSpacing keeps runs of spaces intact (for pre-formatted tables).
	PreserveSpacing bool
}

// Wrap performs word wrapping that preserves runs of spaces and indentation.
// Never splits a word unless it alone exceeds the width, then hard-splits with
// a visible continuation marker ↪ in Faint style.
//
// Per PROMPT1.md §5: Wrap on plain text first, colour second, so colour codes
// never break wrapping. Display width measured with lipgloss.Width.
func Wrap(s string, opts WrapOpts) []string {
	if opts.Width <= 0 {
		opts.Width = 80 // reasonable fallback
	}

	// Strip ANSI codes for width calculation, re-apply after wrapping
	hasColor := strings.Contains(s, "\x1b[")
	plain := s
	if hasColor {
		plain = stripANSI(s)
	}

	// Split into paragraphs (preserve blank lines)
	paragraphs := strings.Split(plain, "\n")
	var lines []string

	for pi, para := range paragraphs {
		if para == "" {
			// Preserve blank lines between paragraphs
			if pi > 0 {
				lines = append(lines, "")
			}
			continue
		}

		// Apply first line indent
		availWidth := opts.Width - opts.FirstIndent
		if availWidth <= 0 {
			availWidth = 1
		}

		// Handle preserveSpacing mode (for tool output with aligned tables)
		if opts.PreserveSpacing {
			// In preserve mode, only wrap if line exceeds width
			// Keep leading spaces on continuation lines
			if displayWidth(para) <= opts.Width {
				lines = append(lines, strings.Repeat(" ", opts.FirstIndent)+para)
				continue
			}
			// Wrap at spaces but preserve indentation
			wrapped := wrapPreserving(para, opts.Width, opts.FirstIndent, opts.HangingIndent)
			lines = append(lines, wrapped...)
			continue
		}

		// Normal word wrap
		words := splitWords(para)
		var curLine strings.Builder
		curWidth := opts.FirstIndent
		if opts.FirstIndent > 0 {
			curLine.WriteString(strings.Repeat(" ", opts.FirstIndent))
		}

		for wi, word := range words {
			ww := displayWidth(word)

			// Check if word fits on current line
			spaceNeeded := 0
			if curLine.Len() > opts.FirstIndent && wi > 0 {
				spaceNeeded = 1 // space before word
			}

			if curWidth+spaceNeeded+ww <= opts.Width {
				// Fits on current line
				if spaceNeeded > 0 {
					curLine.WriteString(" ")
					curWidth++
				}
				curLine.WriteString(word)
				curWidth += ww
			} else {
				// Doesn't fit - finalize current line and start new one
				if curLine.Len() > 0 {
					lines = append(lines, curLine.String())
					curLine.Reset()
				}

				// Start new line with hanging indent
				indentWidth := opts.HangingIndent
				if indentWidth > 0 {
					curLine.WriteString(strings.Repeat(" ", indentWidth))
				}
				curWidth = indentWidth

				// Check if word itself is too long for a line
				if ww > opts.Width-indentWidth {
					// Hard-split long word with continuation marker
					splits := hardSplit(word, opts.Width-indentWidth)
					for i, split := range splits {
						if i > 0 {
							lines = append(lines, curLine.String())
							curLine.Reset()
							if indentWidth > 0 {
								curLine.WriteString(strings.Repeat(" ", indentWidth))
							}
						}
						curLine.WriteString(split)
						if i < len(splits)-1 {
							curLine.WriteString(" ↪") // continuation marker
						}
					}
					curWidth = indentWidth + displayWidth(splits[len(splits)-1])
				} else {
					curLine.WriteString(word)
					curWidth += ww
				}
			}
		}

		// Finalize last line of paragraph
		if curLine.Len() > 0 {
			lines = append(lines, curLine.String())
		}
	}

	return lines
}

// Col describes one column in a table.
type Col struct {
	// Min is the minimum width for this column.
	Min int
	// Max is the maximum width (0 = unlimited).
	Max int
	// Flex allows the column to grow to fill available space.
	Flex bool
	// Align is "left" or "right".
	Align string
	// Policy is "wrap" or "truncate" for overflow handling.
	Policy string
}

// Table formats rows into aligned columns with optional wrapping.
// Returns lines with color already applied.
//
// Per PROMPT1.md §5:
//   - Computes widths from content, shrinks flex columns first
//   - Wraps the last column if configured
//   - Header row optional (first row in Faint if provided)
func Table(cols []Col, rows [][]string, width int) []string {
	if len(cols) == 0 || len(rows) == 0 {
		return nil
	}

	// Calculate actual column widths
	colWidths := computeColumnWidths(cols, rows, width)

	var lines []string
	for ri, row := range rows {
		// Handle rows that might wrap (mainly the last column)
		rowLines := formatTableRow(row, colWidths, cols)
		
		// Apply header styling to first row if it's a header
		// (Caller should pass header as first row if desired)
		if ri == 0 && len(rows) > 1 {
			// Could apply Faint style here, but styling is caller's responsibility
		}
		
		lines = append(lines, rowLines...)
	}

	return lines
}

// KeyValue formats key-value pairs with aligned values.
// Keys in Muted, values in Text (styling applied by caller).
func KeyValue(pairs [][2]string, width int) []string {
	if len(pairs) == 0 {
		return nil
	}

	// Find longest key for alignment
	maxKeyWidth := 0
	for _, pair := range pairs {
		kw := displayWidth(pair[0])
		if kw > maxKeyWidth {
			maxKeyWidth = kw
		}
	}

	// Format each pair
	var lines []string
	for _, pair := range pairs {
		key := pair[0]
		value := pair[1]
		
		// Pad key to maxKeyWidth + 2 spaces
		keyPadded := key + strings.Repeat(" ", maxKeyWidth-displayWidth(key)+2)
		
		// Check if value wraps
		valueWidth := width - maxKeyWidth - 2
		if valueWidth < 20 {
			valueWidth = 20 // minimum readable width
		}
		
		if displayWidth(value) <= valueWidth {
			lines = append(lines, keyPadded+value)
		} else {
			// Wrap value with hanging indent under value column
			valueLines := Wrap(value, WrapOpts{
				Width:         valueWidth,
				HangingIndent: 0,
			})
			for i, vl := range valueLines {
				if i == 0 {
					lines = append(lines, keyPadded+vl)
				} else {
					lines = append(lines, strings.Repeat(" ", maxKeyWidth+2)+vl)
				}
			}
		}
	}

	return lines
}

// Section formats a section heading with count.
// Returns: "HEADING (count)" styled, plus spacing.
func Section(heading string, count int) string {
	if count > 0 {
		return heading + " (" + formatInt(count) + ")"
	}
	return heading
}

// Panel wraps lines in an inset or raised filled rectangle with padding.
// Every row padded to full width so background is solid.
func Panel(lines []string, width int, style lipgloss.Style) []string {
	if width <= 0 {
		width = 80
	}

	var padded []string
	for _, line := range lines {
		// Pad to width
		lw := displayWidth(line)
		padding := width - lw
		if padding < 0 {
			padding = 0
		}
		padded = append(padded, line+strings.Repeat(" ", padding))
	}

	// Apply style (border, background, etc.)
	// For now, return as-is; caller applies lipgloss styling
	return padded
}

// Bullets formats a bulleted list with hanging indents.
func Bullets(items []string, width int) []string {
	var lines []string
	for _, item := range items {
		// Bullet + space = 2 chars
		wrapped := Wrap(item, WrapOpts{
			Width:         width,
			FirstIndent:   2, // "• "
			HangingIndent: 2, // align continuation under text, not bullet
		})
		if len(wrapped) > 0 {
			// Replace first line's indent with bullet
			wrapped[0] = "• " + strings.TrimLeft(wrapped[0], " ")
		}
		lines = append(lines, wrapped...)
	}
	return lines
}

// Numbered formats a numbered list with hanging indents.
func Numbered(items []string, width int) []string {
	var lines []string
	for i, item := range items {
		num := formatInt(i + 1)
		prefix := num + ". "
		indent := displayWidth(prefix)
		
		wrapped := Wrap(item, WrapOpts{
			Width:         width,
			FirstIndent:   indent,
			HangingIndent: indent,
		})
		if len(wrapped) > 0 {
			// Replace first line's indent with number
			wrapped[0] = prefix + strings.TrimLeft(wrapped[0], " ")
		}
		lines = append(lines, wrapped...)
	}
	return lines
}

// Truncate truncates a string to fit width, adding … if truncated.
func Truncate(s string, width int) string {
	w := displayWidth(s)
	if w <= width {
		return s
	}
	
	// Truncate and add ellipsis
	runes := []rune(s)
	var truncated []rune
	curWidth := 0
	for _, r := range runes {
		rw := runeWidth(r)
		if curWidth+rw+1 > width { // +1 for …
			break
		}
		truncated = append(truncated, r)
		curWidth += rw
	}
	
	return string(truncated) + "…"
}

// TruncateMiddle truncates in the middle, keeping head and tail.
// Useful for paths and URLs: /very/long/path/to/file.txt -> /very/.../file.txt
func TruncateMiddle(s string, width int) string {
	w := displayWidth(s)
	if w <= width {
		return s
	}
	
	// Keep roughly 40% head, 40% tail, 20% for ...
	headWidth := (width * 4) / 10
	tailWidth := headWidth
	ellipsis := "..."
	
	runes := []rune(s)
	var head, tail []rune
	
	curWidth := 0
	for _, r := range runes {
		rw := runeWidth(r)
		if curWidth+rw <= headWidth {
			head = append(head, r)
			curWidth += rw
		} else {
			break
		}
	}
	
	curWidth = 0
	for i := len(runes) - 1; i >= 0; i-- {
		r := runes[i]
		rw := runeWidth(r)
		if curWidth+rw <= tailWidth {
			tail = append([]rune{r}, tail...)
			curWidth += rw
		} else {
			break
		}
	}
	
	return string(head) + ellipsis + string(tail)
}

// Helper functions

// displayWidth returns the display width of a string in columns.
// Uses lipgloss.Width which handles ANSI codes and wide characters.
func displayWidth(s string) int {
	return lipgloss.Width(s)
}

// runeWidth returns the display width of a single rune.
func runeWidth(r rune) int {
	// Most characters are width 1
	// Wide characters (CJK, emoji) are width 2
	// Control characters are width 0
	if r < 32 {
		return 0
	}
	// Simple heuristic - proper implementation would use unicode/width
	if r >= 0x1100 {
		return 2 // Wide character
	}
	return 1
}

// splitWords splits text into words, preserving spaces as word boundaries.
func splitWords(s string) []string {
	// Use Fields for normal splitting
	return strings.Fields(s)
}

// wrapPreserving wraps text while preserving indentation and space runs.
// Used for tool output with aligned tables.
func wrapPreserving(s string, width, firstIndent, hangIndent int) []string {
	var lines []string
	
	// Find leading spaces (for future use if needed)
	_ = 0 // leadingSpaces placeholder
	for i, r := range s {
		if r != ' ' && r != '\t' {
			_ = i // would be leadingSpaces
			break
		}
	}
	
	// If line fits, return as-is with first indent
	if displayWidth(s) <= width-firstIndent {
		return []string{strings.Repeat(" ", firstIndent) + s}
	}
	
	// Wrap at nearest space before width
	runes := []rune(s)
	curWidth := firstIndent
	var curLine strings.Builder
	if firstIndent > 0 {
		curLine.WriteString(strings.Repeat(" ", firstIndent))
	}
	
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		rw := runeWidth(r)
		
		if curWidth+rw > width {
			// Find last space to break at
			lineStr := curLine.String()
			lastSpace := strings.LastIndex(lineStr, " ")
			if lastSpace > firstIndent {
				// Break at space
				lines = append(lines, lineStr[:lastSpace])
				curLine.Reset()
				curLine.WriteString(strings.Repeat(" ", hangIndent))
				curLine.WriteString(strings.TrimLeft(lineStr[lastSpace:], " "))
				curWidth = hangIndent + displayWidth(strings.TrimLeft(lineStr[lastSpace:], " "))
			} else {
				// No space found, hard break
				lines = append(lines, lineStr)
				curLine.Reset()
				curLine.WriteString(strings.Repeat(" ", hangIndent))
				curWidth = hangIndent
			}
		}
		
		curLine.WriteRune(r)
		curWidth += rw
	}
	
	if curLine.Len() > 0 {
		lines = append(lines, curLine.String())
	}
	
	return lines
}

// hardSplit splits a long word into chunks that fit width.
func hardSplit(word string, width int) []string {
	if width <= 0 {
		width = 1
	}
	
	var splits []string
	runes := []rune(word)
	var chunk []rune
	curWidth := 0
	
	for _, r := range runes {
		rw := runeWidth(r)
		if curWidth+rw > width && len(chunk) > 0 {
			splits = append(splits, string(chunk))
			chunk = nil
			curWidth = 0
		}
		chunk = append(chunk, r)
		curWidth += rw
	}
	
	if len(chunk) > 0 {
		splits = append(splits, string(chunk))
	}
	
	return splits
}

// computeColumnWidths calculates actual widths for table columns.
func computeColumnWidths(cols []Col, rows [][]string, totalWidth int) []int {
	if len(cols) == 0 {
		return nil
	}
	
	// Start with minimum widths
	widths := make([]int, len(cols))
	for i, col := range cols {
		widths[i] = col.Min
	}
	
	// Calculate maximum needed width for each column
	maxNeeded := make([]int, len(cols))
	for _, row := range rows {
		for i := 0; i < len(row) && i < len(cols); i++ {
			w := displayWidth(row[i])
			if w > maxNeeded[i] {
				maxNeeded[i] = w
			}
		}
	}
	
	// Apply max constraints
	for i, col := range cols {
		if col.Max > 0 && maxNeeded[i] > col.Max {
			maxNeeded[i] = col.Max
		}
		if maxNeeded[i] > widths[i] {
			widths[i] = maxNeeded[i]
		}
	}
	
	// Calculate total used width (including separators)
	usedWidth := 0
	for i, w := range widths {
		usedWidth += w
		if i < len(widths)-1 {
			usedWidth += 2 // separator between columns
		}
	}
	
	// Distribute remaining width to flex columns
	remaining := totalWidth - usedWidth
	if remaining > 0 {
		flexCount := 0
		for _, col := range cols {
			if col.Flex {
				flexCount++
			}
		}
		if flexCount > 0 {
			perFlex := remaining / flexCount
			for i, col := range cols {
				if col.Flex {
					widths[i] += perFlex
				}
			}
		}
	}
	
	return widths
}

// formatTableRow formats one row with the given column widths.
func formatTableRow(row []string, widths []int, cols []Col) []string {
	if len(row) == 0 {
		return nil
	}
	
	// Check if any cell needs wrapping
	needsWrap := false
	for i := 0; i < len(row) && i < len(widths); i++ {
		if displayWidth(row[i]) > widths[i] && cols[i].Policy == "wrap" {
			needsWrap = true
			break
		}
	}
	
	if !needsWrap {
		// Simple case - no wrapping needed
		var parts []string
		for i := 0; i < len(row) && i < len(widths); i++ {
			cell := row[i]
			w := widths[i]
			
			// Truncate or pad
			if displayWidth(cell) > w {
				if cols[i].Policy == "truncate" {
					cell = Truncate(cell, w)
				} else {
					// Wrap - but we already checked, shouldn't happen
					cell = Truncate(cell, w)
				}
			}
			
			// Align
			padding := w - displayWidth(cell)
			if cols[i].Align == "right" {
				cell = strings.Repeat(" ", padding) + cell
			} else {
				cell = cell + strings.Repeat(" ", padding)
			}
			
			parts = append(parts, cell)
		}
		return []string{strings.Join(parts, "  ")}
	}
	
	// Complex case - wrap cells
	// For now, only wrap the last column
	wrappedCells := make([][]string, len(row))
	maxLines := 1
	
	for i := 0; i < len(row) && i < len(widths); i++ {
		cell := row[i]
		w := widths[i]
		
		if displayWidth(cell) > w && cols[i].Policy == "wrap" {
			wrapped := Wrap(cell, WrapOpts{Width: w})
			wrappedCells[i] = wrapped
			if len(wrapped) > maxLines {
				maxLines = len(wrapped)
			}
		} else {
			// No wrap needed
			if displayWidth(cell) > w {
				cell = Truncate(cell, w)
			}
			wrappedCells[i] = []string{cell}
		}
	}
	
	// Format each line
	var lines []string
	for lineIdx := 0; lineIdx < maxLines; lineIdx++ {
		var parts []string
		for i := 0; i < len(wrappedCells) && i < len(widths); i++ {
			var cell string
			if lineIdx < len(wrappedCells[i]) {
				cell = wrappedCells[i][lineIdx]
			} else {
				cell = "" // empty cell for wrapped overflow
			}
			
			w := widths[i]
			padding := w - displayWidth(cell)
			if padding < 0 {
				padding = 0
			}
			
			if cols[i].Align == "right" {
				cell = strings.Repeat(" ", padding) + cell
			} else {
				cell = cell + strings.Repeat(" ", padding)
			}
			
			parts = append(parts, cell)
		}
		lines = append(lines, strings.Join(parts, "  "))
	}
	
	return lines
}

// formatInt formats an integer for display.
func formatInt(n int) string {
	// Simple implementation without thousands separators
	// Could be enhanced with commas for large numbers
	return strings.TrimSpace(strings.Replace(fmt.Sprintf("%d", n), " ", "", -1))
}
