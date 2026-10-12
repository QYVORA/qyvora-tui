package tui

import (
	"github.com/QYVORA/qyvora-tui/internal/layout"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

func (m *model) applyLayout(width, height int) {
	m.geom = m.resolveLayout(width)
	tw := m.transcriptWidth(width)
	m.viewport = viewport.New(max(1, tw), m.viewportHeight(height))
	// The composer spans the whole terminal, not the transcript: it is the one
	// element that addresses the operator, and its text has no indentation to
	// reserve for.
	m.input.Width = max(8, width-lipgloss.Width(m.input.Prompt)-2)
}

// viewportHeight is how many rows the transcript owns between the header band
// and the composer.
func (m *model) viewportHeight(termHeight int) int {
	return max(1, termHeight-m.chromeHeight())
}

// chromeHeight is the number of rows the fixed furniture occupies, so the
// transcript can be given everything that is left.
//
// The header counts as one row because its rule is its border rather than a row
// of its own. The strip is counted whenever an execution exists, whether or not
// it currently has anything to say, so the viewport does not resize every time
// follow mode toggles: a viewport that changes height on a scroll moves the
// transcript under the fingers using it.
//
// An open form is measured rather than assumed. A form stands in for the
// composer but can be several rows taller than it, and charging it the
// composer's two rows is how a long form pushes its own fields off the top of
// the terminal.
func (m *model) chromeHeight() int {
	rows := m.headerHeight()
	if len(m.blocks) > 0 {
		rows++ // the strip between transcript and composer
	}
	if m.form != nil {
		return rows + m.formHeight()
	}
	return rows + 3 // the composer's rule, mode bar and input
}

// headerHeight is how many rows the header band occupies: one, or the height of
// the banner when the tool supplied one.
//
// The banner is charged to the transcript here, which is what keeps a tall
// wordmark from pushing the composer's input row off the bottom of a short
// terminal. Chrome is the difference between the terminal and the viewport, so
// anything drawn in the header has to be counted in it or the viewport is built
// taller than the space it was given.
func (m model) headerHeight() int {
	if h := m.bannerHeight(); h > 0 {
		return h
	}
	return 1
}

// formHeight is the number of rows an open form draws.
func (m model) formHeight() int {
	f := m.form
	if f == nil {
		return 0
	}
	rows := 3 + len(f.fields) // its rule, mode bar, title, one row per field
	if f.Err() != "" {
		rows++
	}
	return rows + 1 // the key hint footer
}

// rebox reflows the viewport at the dimensions the current layout grants, on a
// copy of the model. The scroll position the operator set is preserved:
// SetContent reflows the history at the new width and clamps YOffset to the
// new total, and following models stay pinned to the bottom.
func (m model) rebox() model {
	tw, th := m.transcriptWidth(m.width), m.viewportHeight(m.height)
	if m.viewport.Width == tw && m.viewport.Height == th {
		return m
	}
	m.viewport.Width = tw
	m.viewport.Height = th
	m.refreshViewport()
	return m
}

// capabilitiesRegion builds the capability region, or nil when there is nothing
// to put in it.
//
// Returning nil rather than an empty region is the point: the layout is told
// there is a navigation region only when one has content, so a tool with no
// registry never gets a column of nothing.
//
// Each capability takes two lines: the name, then its detail indented beneath
// it. The region is 16 to 24 columns wide, and a name like "Profile target
// baseline" uses all of it, so a single line cannot carry both the name and the
// facts that change a decision. Two lines can, and the alternative -- a row of
// truncated fragments -- is what makes a narrow column unreadable.
func (m model) resolveLayout(width int) layout.Layout {
	l := layout.LayoutFor(width, m.height, layout.LayoutOptions{
		Navigation: m.showCapabilities && m.caps != nil && len(m.caps.Items) > 0,
		// The executions region exists for the whole session after the first
		// command, not just while a run is in flight: a finished run's history
		// is exactly what the navigator is for. This is what keeps the region
		// from vanishing the moment a command completes.
		Activity: len(m.blocks) > 0,
	})
	// A region that turned out to have no content gives its width back, so the
	// transcript is not left narrower than it needs to be.
	if l.Navigation > 0 && m.capabilitiesRegion(l.Navigation) == nil {
		l = layout.LayoutFor(width, m.height, layout.LayoutOptions{Activity: len(m.blocks) > 0})
	}
	if l.Activity > 0 && m.executionsRegion(l.Activity) == nil {
		l = layout.LayoutFor(width, m.height, layout.LayoutOptions{
			Navigation: m.showCapabilities && m.caps != nil && len(m.caps.Items) > 0,
		})
	}
	return l
}

// transcriptWidth is the width the viewport is built at: the layout's share, or
// the whole terminal when no region is drawn.
func (m model) transcriptWidth(width int) int {
	if !m.geom.ShowRegions {
		return max(1, width)
	}
	return max(1, m.geom.Transcript)
}
