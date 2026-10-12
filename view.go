package tui

import (
	"strings"

	"github.com/QYVORA/qyvora-tui/internal/layout"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// tree glyphs, matching the shape a reader expects from a file tree.
//
// These are used only where there is real nesting to show, which after the
// redesign is the event log. Findings and artifacts are flat lists, and drawing
// branches for a list tells the reader there is a hierarchy that is not there.
const (
	treeBranch = "├─"
	treeLeaf   = "└─"
)

// gutterRail is the vertical rule that runs down the left of a command's body.
const gutterRail = "│"

// The columns a finding list is built from.
//
// A finding is a severity and a target with a sentence between them, and the
// sentence is what explains the other two. The severity is therefore given the
// width of its longest real word ("CRITICAL") and the target a bounded share of
// what is left, so every target in a run starts in the same column and the
// titles -- the part carrying the meaning -- get the width that remains.
//
// Nothing here is fixed in absolute terminal columns. The title column grows
// with the terminal instead, because a 34-character title cap left most of a
// wide terminal empty and truncated titles mid-word while doing it.
const (
	// severityColWidth is the width of the severity column, set to the longest
	// canonical severity word.
	severityColWidth = 8

	// severityRail is the coloured rule before each finding. It carries the
	// severity to the eye before the word is read, and it is a solid block
	// rather than a glyph so it still reads as a rail with colour switched off.
	severityRail = "▌"

	// minFindingTitle keeps a title readable when the terminal is narrow. Below
	// this the target is given up before the title is squeezed further.
	minFindingTitle = 18

	// The bounds on the target column. A target is an address, so it is worth
	// reserving room for it, but it is not worth more than a third of the line:
	// a very long address truncated to a share is no more useful than one
	// truncated to a column.
	minFindingTarget = 12
	maxFindingTarget = 30
)

// View renders the whole interface: a compact header, the scrolling session,
// and the composer pinned to the bottom.
//
// The terminal session is the workspace. There is no panel grid and no
// permanent dashboard: the middle of the screen is history, and the only fixed
// furniture is the prompt the user types into.
func (m model) View() string {
	if m.quitting {
		return ""
	}
	if m.width <= 0 || m.height <= 0 {
		// Before the first resize there is no geometry to lay out against;
		// render the composer alone rather than guessing at widths.
		return m.renderComposer()
	}

	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n")

	// The viewport takes everything between the header band and the composer.
	// It scrolls independently, so history stays reachable while a command runs.
	//
	// Regions are drawn only when the layout granted them width *and* there is
	// something to put in them. A region with no content is not rendered at
	// all: an empty column beside a working session is worse than the session
	// being wider.
	// The layout is resolved here, from the current state, rather than read
	// from whatever the last resize decided. State can change between a resize
	// and a render -- a run starting, the operator pressing F1 -- and resolving
	// here is what keeps the region widths and the drawn regions from
	// disagreeing.
	//
	// This is the pure half of applyLayout. View is a value method, so anything
	// done here is discarded by the time the program's model carries on: the
	// rebox below reflows a copy of the viewport and cannot disturb the real
	// widget or the scroll position the operator set.
	m.geom = m.resolveLayout(m.width)

	// The viewport may still sit at the dimensions of an earlier layout: a
	// block was appended, a region toggled, the terminal resized. Reflow it on
	// this copy so the mid column is exactly as wide as the layout granted the
	// transcript -- otherwise composeRegions trims lines back onto "…".
	m = m.rebox()

	// Both regions share the transcript's row budget. A panel drawn taller than
	// the transcript beside it has its last rows below the terminal, where they
	// are never seen: the newest run, which is the one the operator is watching,
	// is exactly the row that disappears.
	rows := m.viewport.Height
	left := m.capabilitiesRegion(m.geom.Navigation)
	right := m.executionsRegion(m.geom.Activity)
	if left != nil {
		left.Height = rows
	}
	if right != nil {
		right.Height = rows
	}

	// The widths come from the layout, and the regions are rendered at exactly
	// the widths it granted them, so the columns tile the terminal.
	// A boxed region is two columns wider than the layout granted it: the rule
	// and the space beside it. The separator between columns is the composer's,
	// so it is not counted here. Both numbers are the ones layout.go charged the
	// transcript for, which is what keeps the two from disagreeing.
	//
	// A width is taken from the rendered text rather than from the region
	// pointer, because a region can exist and still draw nothing: too short to
	// render, or empty. Charging the transcript for a panel that is not on screen
	// is how the columns end up over-running the terminal.
	// The transcript's row count is recorded as it is drawn. paintFrame needs to
	// know which rows are the transcript to put them on the Base field, and
	// counting rows by arithmetic from the frame's edges would be a second
	// version of chromeHeight that can disagree with the first.
	transcriptRows := m.viewport.Height

	leftBox, rightBox := boxedRegion(left, m.theme, -1), boxedRegion(right, m.theme, 1)
	navW, actW := 0, 0
	if leftBox != "" {
		navW = m.geom.Navigation + layout.RegionChrome
	}
	if rightBox != "" {
		actW = m.geom.Activity + layout.RegionChrome
	}
	switch {
	case navW > 0 && actW > 0:
		b.WriteString(composeRegions(m.viewport.View(), m.geom.Transcript,
			leftBox, navW, rightBox, actW, m.width))
	case navW > 0:
		b.WriteString(composeRegions(m.viewport.View(), m.geom.Transcript,
			leftBox, navW, "", 0, m.width))
	case actW > 0:
		b.WriteString(composeRegions(m.viewport.View(), m.geom.Transcript,
			"", 0, rightBox, actW, m.width))
	default:
		b.WriteString(m.viewport.View())
	}
	b.WriteString("\n")

	// The row between the transcript and the composer is the session's pulse:
	// while the operator is following the newest output it is silent, and the
	// moment they scroll back it reports how much has appeared below the fold.
	if len(m.blocks) > 0 {
		// The strip's row is reserved in chromeHeight whether or not it has
		// anything to say, so the viewport does not resize as follow mode
		// toggles. It is written as a full-width row even when silent: an empty
		// string here would be a row of zero columns in the middle of a frame
		// whose rows are all the terminal's width, which is a ragged edge the
		// eye reads as a rendering fault rather than as a blank row.
		b.WriteString(padTo(m.renderStrip(), m.width))
		b.WriteString("\n")
	}

	var composer string
	if m.form != nil {
		// The form replaces the composer while it is open. Two input lines at
		// once would leave the operator unsure which one their keystrokes are
		// going to.
		composer = m.renderForm()
	} else {
		composer = m.renderComposer()
	}
	// The composer block grounds itself: each of its rows carries the fill it
	// should have and re-establishes it after every reset the row contains.
	// paintFrame must therefore leave the block alone -- laying the frame's
	// ground over it would clear the field's fill at the first reset the input
	// text emits, which is exactly the "coloured text instead of a field" bug.
	// The block is always the last rows of the frame, so it is counted from the
	// end rather than located by searching the frame.
	b.WriteString(composer)
	frame := strings.Split(b.String(), "\n")
	selfGrounded := strings.Count(composer, "\n") + 1
	return m.paintFrame(frame, transcriptRows, selfGrounded)
}

// paintFrame puts the whole frame on the tool's own ground.
//
// The palette is a ladder of four fills, and a ladder only means anything if the
// steps are used in order: Inset is the field behind everything, and each lighter
// step is a surface raised off it. Without this the chrome is painted and the
// transcript is left on whatever background the terminal happens to have, so
// Surface has nothing to be raised above and the interface reads as a set of
// floating cards rather than one surface with panels on it.
//
// Painting is done here rather than at each call site because the ground has to
// reach the parts nothing draws: the empty rows above a short transcript, the
// columns between the panels, the row of the frame that holds no content at all.
// Those are the majority of the pixels on an idle screen, and they are exactly the
// ones that would otherwise show through as the terminal's own colour.
//
// The ground goes down first and the panels paint over it, so a row is painted
// twice but never twice in the wrong order: Inset covers the frame, and the
// transcript and panels that already carry their own fills replace it.
//
// selfGrounded is the number of trailing rows that ground themselves and must be
// left untouched: the composer and the form, whose rows carry a fill (Raised for
// the input field, Surface for the focused form row) and re-establish it after
// every reset. Grounding those rows with Inset here would overwrite that fill at
// the first reset they contain, which is the bug this parameter exists to avoid.
func (m model) paintFrame(rows []string, transcriptRows, selfGrounded int) string {
	if !m.theme.Color {
		return strings.Join(rows, "\n")
	}
	first := len(rows) - selfGrounded
	painted := make([]string, 0, len(rows))
	for i, row := range rows {
		if i >= first {
			painted = append(painted, row)
			continue
		}
		row = ground(m.theme.Inset, row, m.width)
		// The transcript is the field the work happens on: one step above the
		// frame, so the panels beside it read as raised off it. It starts below
		// the header and runs for the rows the viewport was boxed to.
		if i > 0 && i <= transcriptRows {
			row = ground(m.theme.Base, row, m.width)
		}
		painted = append(painted, row)
	}
	return strings.Join(painted, "\n")
}

// renderForm draws the open form's fields.
//
// Only the field being edited carries a cursor. Every field shows its value
// plainly, so a form is readable at a glance and does not need to be in edit
// mode to be reviewed.
