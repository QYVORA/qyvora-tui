package tui

// The layout breaks at the widths where the session can still be read.
//
// These are not round numbers chosen for tidiness. Each marks a point where
// adding a region stops being free:
//
//	below 60   a region costs more than it shows. The transcript is the
//	           interface and everything else is a detail view behind a key.
//	60 to 120  there is room for a sidebar or a detail column, but not for
//	           two, and only when the tool has something to put in them.
//	above 120  a full multi-region shell is affordable, and the activity
//	           stream is worth watching while it runs.
//
// The rule underneath all three: a small terminal stays simple and a large one
// gains density. Nothing here forces a three-column layout onto a tool that has
// three commands, and the narrow case is the existing single column rather than
// a degraded version of something wider.
const (
	// CompactWidth is the narrowest width at which a side region is worth
	// drawing.
	//
	// It is derived rather than picked, because it is exactly the width at which
	// the narrowest permitted region can coexist with a transcript that is still
	// readable. Writing it as a round number is how it drifts: the region floor
	// rose to 28 and a hand-written 60 stopped meaning anything, granting a
	// panel that pushed the transcript below its minimum. The layout then took
	// the panel away and reported a wide layout with no regions, which is the
	// confusing outcome this avoids.
	CompactWidth = minRegionWidth + regionChrome + regionGap + minTranscriptWidth

	// WideWidth is the width at which a full multi-region shell becomes
	// affordable: both panels beside a transcript wide enough that the left one
	// is not squeezing the words out of it.
	//
	// This one stays a chosen number rather than a derived one, because it is
	// not a limit: two panels beside a usable transcript fit well before it. It
	// is the point past which a second panel is worth having at all. Narrower
	// than this and the transcript would be readable but cramped, and a cramped
	// transcript is worse than a single panel beside a comfortable one.
	WideWidth = 120
)

// LayoutMode is how much structure the interface can afford at a given width.
type LayoutMode int

const (
	// ModeCompact is a single column: header, transcript, composer. The
	// existing design, and the fallback when a region would not fit.
	ModeCompact LayoutMode = iota

	// ModeAdaptive can carry one side region beside the transcript.
	ModeAdaptive

	// ModeWide can carry navigation and activity either side of the
	// transcript.
	ModeWide
)

// String names the mode, for the status hint and for tests.
func (m LayoutMode) String() string {
	switch m {
	case ModeAdaptive:
		return "adaptive"
	case ModeWide:
		return "wide"
	default:
		return "compact"
	}
}

// Layout is the resolved geometry for one terminal size.
//
// It is computed once per resize and then read by the shell and the components,
// so a component never has to ask "how wide is the terminal" and answer it
// differently from its neighbour.
type Layout struct {
	// Width and Height are the terminal dimensions the layout was resolved
	// for.
	Width  int
	Height int

	// Mode is the structure the width affords.
	Mode LayoutMode

	// Navigation is the width of the navigation column, or zero when the
	// layout has none. Zero is meaningful: a region that does not fit is
	// absent, not squeezed.
	Navigation int

	// Activity is the width of the activity column, or zero when the layout has
	// none.
	Activity int

	// Transcript is the width left for the session itself. It is always
	// positive in a usable layout; a layout that cannot give the transcript
	// room collapses to compact instead.
	Transcript int

	// ShowRegions reports whether any side region is drawn. A tool with no
	// capabilities and no live activity still gets a full-width transcript
	// even on a wide terminal, because empty columns are worse than none.
	ShowRegions bool
}

// Region widths. A region that is too narrow to carry a label and a value is
// not worth drawing, so each has a floor and a ceiling.
const (
	// minRegionWidth is the narrowest a side panel is drawn at. It is also the
	// width below which a region is not a region at all: NewRegion clamps up to
	// it and Region.Render refuses anything narrower, so the floor is a single
	// number rather than one per region.
	//
	// 28 rather than the 20 this used to allow, because 20 columns cannot carry
	// a capability name and its detail on the same line and forcing them onto
	// separate wrapped lines is what made the left panel read as a list of
	// fragments.
	minRegionWidth = 28

	// maxRegionWidth is the widest a side panel grows. Past this a panel stops
	// being a panel and becomes the interface, and the transcript it was
	// accompanying is now the accessory.
	maxRegionWidth = 36
)

// minTranscriptWidth is the narrowest a transcript is still readable at. Below
// it, no region is granted: the session is the interface, and the side panels
// exist to support it.
//
// The number comes from the content rather than from taste. Tool output is
// indented, tree-drawn and prefixed with a status glyph, and the common lines
// the fleet prints -- a finding, a stage transition, an argument echo -- run to
// around 36 columns before they wrap. A transcript narrower than that wraps
// every line, which is the failure the side regions must not cause.
const minTranscriptWidth = 36

// minRegionHeight is the shortest terminal a region is drawn on. A 24-column
// panel beside a four-line transcript adds nothing a person can use.
const minRegionHeight = 8

// The columns a region costs beyond its own width.
//
// A boxed region is drawn as a rule, a space and its content, so it occupies two
// more columns than the width the layout granted it, and every column is
// followed by a single space separating it from the next. Both are charged here,
// once, and both are read back by the composer, because a layout that accounts
// for the region's columns but not for its rule hands the difference to the
// transcript -- which then clamps every line and fills the panel's space with an
// ellipsis.
const (
	regionChrome = 2
	regionGap    = 1
)

// regionColumns is the terminal width a set of regions occupies once each is
// boxed and separated. A region granted no width is not drawn and costs nothing.
func regionColumns(widths ...int) int {
	total := 0
	for _, w := range widths {
		if w > 0 {
			total += w + regionChrome + regionGap
		}
	}
	return total
}

// LayoutFor resolves the layout for a terminal size.
//
// The rules, in order:
//
//   - Below CompactWidth, compact, unconditionally. At that size the transcript
//     is all the operator can use, and taking any of it for a panel would leave
//     something unreadable.
//   - Between CompactWidth and WideWidth, the executions region only. It is the
//     one panel that earns its columns on merit: it changes while the operator
//     watches, and the capability registry does not.
//   - At WideWidth, both regions when both have content.
//
// The asymmetry between the two panels below WideWidth is deliberate rather
// than an oversight. The registry is reference material -- useful, static, and
// available on F1 in two keystrokes -- so a column of it beside a live session
// is a claim on the transcript that the session can use better. Executions are
// the session: the run in flight and the runs behind it, which cannot be
// reproduced on demand from a keypress. When the two compete, the transcript
// goes to the one that cannot wait.
//
// Every grant is conditional on the transcript keeping minTranscriptWidth
// columns. That single condition is what stops a 70-column terminal being
// carved into a 20-column transcript and two useless panels: a region is
// dropped rather than allowed to starve the thing it was meant to accompany.
func LayoutFor(width, height int, opts LayoutOptions) Layout {
	l := Layout{Width: width, Height: height, Mode: ModeCompact, Transcript: max(1, width)}
	if width < CompactWidth || height < minRegionHeight {
		return l
	}

	wide := width >= WideWidth
	// The registry only earns a column on a terminal wide enough to carry both
	// panels and the words between them.
	nav := 0
	if wide && opts.Navigation {
		nav = regionFor(true, regionWidth(width), width)
	}
	act := regionFor(opts.Activity, regionWidth(width), width)

	// Two regions need the wide breakpoint and room for both plus a usable
	// transcript. Below that there is room for one, and it is the executions
	// region.
	both := nav > 0 && act > 0 && fits(width, nav, act)
	// With no run in flight there is no executions region to show, so a wide
	// terminal falls back to the registry rather than to an empty column.
	if act > 0 && !both {
		nav = 0
	}

	switch {
	case both:
		l.Navigation, l.Activity = nav, act
		l.Transcript = width - regionColumns(nav, act)
		l.Mode = ModeWide
	case act > 0:
		l.Activity = act
		l.Transcript = width - regionColumns(act)
		l.Mode = ModeAdaptive
	case nav > 0:
		l.Navigation = nav
		l.Transcript = width - regionColumns(nav)
		l.Mode = ModeAdaptive
	default:
		// Wide enough, but there is nothing to show. A wide terminal with no
		// regions is not a degraded layout; it is the right one.
		l.Mode = ModeWide
		return l
	}
	l.ShowRegions = l.Navigation > 0 || l.Activity > 0
	return l
}

// regionFor decides one region's width, returning zero when it is not granted.
//
// A region is granted only if the transcript is still usable without it. That
// single condition is what stops a 70-column terminal being carved into a
// 20-column transcript and two useless panels: a region is dropped rather than
// allowed to starve the thing it was meant to accompany.
func regionFor(want bool, region, width int) int {
	if !want || !fits(width, region) {
		return 0
	}
	return region
}

// fits reports whether the given regions leave the transcript usable.
func fits(width int, regions ...int) bool {
	if len(regions) == 0 {
		return false
	}
	for _, r := range regions {
		if r <= 0 {
			return false
		}
	}
	return width-regionColumns(regions...) >= minTranscriptWidth
}

// regionWidth is the width both side regions are drawn at.
//
// A quarter of the terminal, four columns of margin, clamped to the region
// bounds. The clamp does the real work at both ends: without a floor, a narrow
// terminal would grant a panel too narrow to read; without a ceiling, a wide one
// would let the panel take over. The share alone grows without limit, and a
// panel that grows without limit stops being a panel.
//
// Both regions take the same number so the two columns are symmetric and the
// transcript's width is the same whichever one is showing. They are different
// content and would each prefer a different width, but a layout that sizes them
// differently depending on which is present makes the transcript jump sideways
// when a region opens and closes.
func regionWidth(width int) int {
	return clampInt((width-regionMargin)/4, minRegionWidth, maxRegionWidth)
}

// regionMargin is the four columns held back from the region share. The right
// two are the composer's own margin, and the left two are what keeps a panel's
// outer edge off the terminal edge, where it reads as a clipped column rather
// than as a deliberate border.
const regionMargin = 4

// LayoutOptions is what the shell knows about the tool when it resolves a
// layout. Regions are offered, not imposed: a tool with no capability registry
// and no event stream should not be given empty columns.
type LayoutOptions struct {
	// Navigation is true when the tool has something to navigate.
	Navigation bool

	// Activity is true when there is a live event stream to show.
	Activity bool
}

// clampInt bounds a value between lo and hi.
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
