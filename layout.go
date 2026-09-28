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
	CompactWidth = 60

	// WideWidth is the width at which a full multi-region shell becomes
	// affordable.
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
// not worth drawing, so each has a minimum and a ceiling.
const (
	minNavigationWidth = 20
	minActivityWidth   = 24
	maxNavigationWidth = 30
	maxActivityWidth   = 30
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
//   - Between CompactWidth and WideWidth, at most one region. Activity is
//     preferred, because watching work happen is what a wider terminal can
//     uniquely add, but navigation is taken when there is no run in flight.
//   - At WideWidth, both regions when both have content.
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

	nav := regionFor(opts.Navigation, regionWidth(width, minNavigationWidth, maxNavigationWidth), width)
	act := regionFor(opts.Activity, regionWidth(width, minActivityWidth, maxActivityWidth), width)

	// Two regions need the wide breakpoint and room for both plus a usable
	// transcript. Below that there is room for one, and activity takes it: it
	// changes while the operator watches, and the registry does not.
	both := nav > 0 && act > 0 && width >= WideWidth && fits(width, nav, act)
	// Navigation yields only when it is actually competing with activity for the
	// space. With no run in flight it is the only candidate, and there is no
	// reason to discard it.
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

// regionWidth scales a region against the terminal.
//
// The share is a quarter of the space above the transcript's minimum, clamped to
// the region's own bounds: a side panel that grows without limit stops being a
// panel and starts being the interface.
func regionWidth(width, min, max int) int {
	share := (width - minTranscriptWidth) / 4
	return clampInt(share, min, max)
}

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
