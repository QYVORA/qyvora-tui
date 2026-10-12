package tui

import (
	"fmt"
)

func (m model) capabilitiesRegion(width int) *Region {
	if width <= 0 || !m.showCapabilities || m.caps == nil || len(m.caps.Items) == 0 {
		return nil
	}
	r := NewRegion("CAPABILITIES", width)
	r.Empty = "none published"
	r.Height = m.capabilitiesBudget()
	return m.fillCapabilitiesRegion(r)
}

// capabilitiesBudget is how many rows the capability region may draw, title and
// rule included.
//
// The budget comes from viewportHeight rather than from a guess at the terminal
// height. The region is drawn beside the transcript and shares its rows, so that
// is the number that is actually true at draw time, and computing it from the
// same function the viewport uses is what keeps the two from disagreeing. The
// previous guess of height-10 meant a short terminal hid capabilities that would
// have fitted, and then reported a "+N more" count unrelated to how many were
// really hidden.
func (m model) capabilitiesBudget() int {
	h := m.viewportHeight(m.height)
	if h <= chromeRows {
		return 0
	}
	return h
}

// fillCapabilitiesRegion adds as many capabilities as the region's budget
// allows, complete ones only, and says how many were left out.
//
// A capability takes its name, then its detail indented beneath it, then its
// note. Rows that do not fit are not drawn half: a row cut off by the region's
// bottom edge is worse than one capability fewer, because the operator cannot
// tell an incomplete entry from a complete one.
func (m model) fillCapabilitiesRegion(r *Region) *Region {
	entries := m.caps.Entries()
	// chromeRows is subtracted because the title and the rule are part of the
	// budget and are drawn before any content.
	budget := r.Height - chromeRows
	if r.Height <= 0 {
		budget = 0
	}
	used, shown := 0, 0
	// When anything is left out, the last row is spent saying so. Reserving it up
	// front rather than adding it afterwards is what makes the notice reliable: a
	// loop that fills the budget and only then tries to append "+N more" has
	// already used the row, so the notice is the thing that gets dropped and the
	// count of hidden capabilities goes unmentioned.
	moreRow := 1
	for _, e := range entries {
		rows := capabilityRows(e)
		if budget > 0 && used+rows+moreRow > budget {
			break
		}
		marker, style := "●", m.theme.Value
		switch {
		case !e.Available:
			// An unavailable capability is dimmed and says why, so the region
			// explains the absence instead of looking like an oversight.
			marker, style = "○", m.theme.Detail
		case e.Note == "live provider":
			style = m.theme.Warning
		}
		// The style is applied here rather than only chosen: a dimmed row that
		// renders identically to a live one is not dimmed, and the distinction
		// the marker makes is the only thing left carrying it.
		r.Add(style.Render(clampLine(marker+" "+e.Name, r.Width-2)))
		if e.Detail != "" {
			r.Add(m.theme.Detail.Render(clampLine("  "+e.Detail, r.Width-2)))
		}
		if e.Note != "" && e.Note != "live provider" {
			r.Add(m.theme.Hint.Render(clampLine("  "+e.Note, r.Width-2)))
		}
		used += rows
		shown++
	}
	if remaining := len(entries) - shown; remaining > 0 {
		// The count is what was actually left out, which is only true now that
		// the loop stops on the budget rather than on a guess.
		r.Add(m.theme.Hint.Render(clampLine(fmt.Sprintf("  +%d more", remaining), r.Width-2)))
	}
	return r
}

// capabilityRows is how many rows one capability occupies: its name, its detail
// and its note, counting only the ones it actually has.
//
// Counting it rather than assuming two is what keeps the last capability from
// being drawn when only half of it fits.
func capabilityRows(e Entry) int {
	n := 1
	if e.Detail != "" {
		n++
	}
	if e.Note != "" && e.Note != "live provider" {
		n++
	}
	return n
}

// boxedRegion renders a region with its inner rule, or an empty string when
// there is nothing to draw.
//
// The empty string is the signal, not a special case: a caller that charges the
// transcript for a column has to know whether the panel is on screen, and a
// region that renders nothing is not on screen.
func boxedRegion(r *Region, t Theme, side int) string {
	if r == nil {
		return ""
	}
	return r.Boxed(t, side)
}

// executionsRegion builds the executions region: the session's navigator.
//
// It is the persistent right-hand column once any command has run: the running
// command at the top with its live counts, the finished runs beneath it, and
// each row expandable. It replaces the old single-purpose activity view,
// because a run finishing is exactly when its history becomes useful, and a
// column that vanished at that moment would be hiding the thing it had shown.
//
// A width of zero means the layout granted no region, and returning a region
// anyway would charge the transcript columns for a panel that draws nothing.
func (m model) executionsRegion(width int) *Region {
	if width <= 0 || len(m.blocks) == 0 {
		return nil
	}
	r := NewRegion("EXECUTIONS", width)
	r.Focused = m.regionFocus
	r.Empty = "no executions"
	entries := m.execEntries(width)
	if len(entries) == 0 {
		return r
	}
	rows, more := m.expandedRegionWindow(entries)
	for _, line := range rows {
		r.Add(line)
	}
	if more {
		r.Add(m.theme.Hint.Render(clampLine("  ↓ scroll for older", width)))
	}
	return r
}

// resolveLayout resolves the geometry for a width from the model's current state.
//
// It is pure: it reads the model and returns a Layout, changing nothing. Both
// the resize path and the render path use it, which is what stops the region
// widths and the drawn regions from being computed from two different pictures
// of the model.
//
// A region is offered only when it will actually be drawn: the tool has
// something to put in it, and the operator has asked for it. Offering a region
// that then renders nothing reserves a column for a blank, which is the outcome
// the whole rule exists to prevent.
