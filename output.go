package tui

func (m model) renderOutput(b *block, expanded bool) []string {
	if len(b.output) == 0 {
		return nil
	}
	if !expanded {
		note := plural(len(b.output), "line", "lines")
		if b.outputOmitted {
			note += " shown (tail kept)"
		}
		// An affordance, not a section. It is dimmed and set further in than a
		// heading so it reads as a note attached to the run rather than as the
		// first line of its output.
		//
		// The keys it names are the ones that actually work from where the
		// operator is. Expanding is done from the executions region, so F2 on
		// its own only moves focus there; saying "F2 expand" sent people
		// pressing a key that visibly did nothing. With the region already
		// focused the step is not needed and is not named.
		keys := "· F2 focus, enter expand"
		if m.regionFocus {
			keys = "· enter expand"
		}
		return []string{"   " + m.theme.Detail.Render("output "+note) + " " +
			m.theme.Hint.Render(keys)}
	}
	// A printed block is quoted under a rule rather than mixed into the tree:
	// the tree is for results the interface understood, and this is the tool
	// speaking for itself.
	lines := []string{"  " + m.theme.Heading.Render("OUTPUT")}
	if b.outputOmitted {
		lines = append(lines, "   "+m.theme.Detail.Render("(truncated to the last "+plural(b.capLimit(), "line", "lines")+")"))
	}
	width := m.contentWidth() - 2
	for _, raw := range b.output {
		// Wrap first, highlight second. Highlighting before wrapping would put
		// escape sequences into the string the wrapper measures, and it would cut
		// one in half at the break point -- the wrapped line then paints the rest
		// of the terminal the colour it was meant to end in.
		for _, line := range wrapText(raw, width) {
			lines = append(lines, "  "+highlightLine(line, m.theme))
		}
	}
	return lines
}

// wrapText breaks a line at width, preferring spaces so words stay intact.
//
// The spaces inside the line are content, not separators. A run of them is the
// gap between two columns, and collapsing it -- which is what a
// strings.Fields-based wrap does -- turns an aligned table into prose and a
// tree drawing into a staircase. So the runs are carried through verbatim and a
// line that already fits is returned untouched.
//
// Wrapping is the one place where exactness is impossible: a line too wide for
// the terminal has to become several. Where it breaks, it breaks at a space, and
// the spaces at the break are consumed rather than carried to the next line. A
// continuation does not inherit the original indentation, because the caller has
// already indented every line it draws and a second indent would be a level of
// depth the source never had.
//
// A word longer than the line is split by hand rather than dropped. Losing
// content silently is worse than an awkward break.
