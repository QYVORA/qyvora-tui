package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// BaseTheme is the QYVORA palette every tool inherits unless it supplies its
// own.
//
// The ground is a green-black ladder rather than a neutral grey: the accent is
// the QYVORA green, and a neutral ground under a saturated accent reads as two
// different products. Four steps, darkest to lightest, each far enough above the
// last that the layering still separates on a panel only a few rows tall.
//
// Colour is rationed the same way it always was. A terminal agent spends most
// of its life showing text, and colouring all of it is how a session becomes
// unreadable. What has changed is what the ration is spent on: the accent now
// carries the prompt, the mode bar and identity, and severity has a real scale
// rather than three words in three shades of the same hue.
var BaseTheme = Palette{
	// Ground, darkest to lightest. surfaceLadder holds the order these four
	// are asserted in, and TestSurfaceLadderIsMonotonic asserts that each step
	// is at least 1.15:1 against the one below it. That is a stricter rule than
	// it sounds, because near black the contrast formula is dominated by its own
	// 0.05 floor, and a ladder that only looks like it has steps in a screenshot
	// is a ladder that has none on a dim terminal. These four are the darkest
	// values that satisfy it while staying green-black rather than neutral grey.
	Inset:   "#020302",
	Base:    "#0F1D16",
	Surface: "#182D22",
	Raised:  "#1F3B2D",

	// Structure. These sit just above the ground on purpose: a rule is not
	// text, and a divider that competes with the words it separates is worse
	// than no divider. Rule and Border are held to a contrast band rather than
	// a floor, because they have to be visible without being legible.
	Rule:   "#274938",
	Border: "#2D5541",

	// Text. Text, Muted and Faint are the only inks the interface sets on the
	// ground, and each one clears the bar its job requires. Faint clears 3:1,
	// not 4.5:1, because its job is a timestamp and a count, not a reading.
	Text:  "#E9F7EF",
	Muted: "#93AC9F",
	Faint: "#6F8378",

	// Identity. Accent is the resting state; Hi and Lo are its two ends, used
	// for the leading cell of a progress bar and for de-emphasised accent text
	// respectively. Bg is the field a pill sits on.
	Accent:   "#22E58B",
	AccentHi: "#6BFFB0",
	AccentLo: "#12B76A",
	AccentBg: "#0E3B26",

	// Severity and status. These are deliberately not part of the branding
	// surface: a tool overriding its own failure colour would make a failure
	// stop reading as a failure. CriticalBg is the one saturated field in the
	// palette, spent on the one label that has to be the loudest thing on
	// screen.
	Info:       "#4DA6FF",
	InfoBg:     "#10263F",
	Cyan:       "#35D4DE",
	Violet:     "#B794F6",
	Warning:    "#FFC93C",
	High:       "#FF9340",
	Danger:     "#FF6E85",
	CriticalBg: "#D7263D",
	Cancelled:  "#949FAA",

	// PillText is not a themeable slot. CRITICAL is the only label allowed to
	// sit on a saturated fill, and white is the only ink that stays legible on
	// it, so it is fixed rather than offered to a tool that could get it wrong.
	PillText: "#FFFFFF",
}

// Palette is the semantic colour set a theme is written in.
//
// A tool fills in only the slots it means to change; everything else falls back
// to the base. That is what makes a partial theme legitimate: a tool with one
// recognisable brand colour should be able to set Accent and inherit the rest,
// rather than restating thirty hex values to get one of them right.
//
// A slot may be empty after resolution when the terminal cannot render it. An
// empty background slot means "paint no fill", which is the correct state at
// sixteen colours rather than a missing value.
type Palette struct {
	// The ground, darkest to lightest. These are painted, so a component
	// distinguishes them only by their being different.
	Inset   string
	Base    string
	Surface string
	Raised  string

	// Rule and Border draw structure. They are foreground colours, not fields.
	Rule   string
	Border string

	// Accent is the interface's identity colour: prompt, rules, success.
	Accent string
	// AccentHi, AccentLo and AccentBg are the ends of the accent ramp and the
	// field a pill sits on.
	AccentHi string
	AccentLo string
	AccentBg string
	// Warning, Danger, Cancelled and Info carry execution state.
	Warning   string
	Danger    string
	Cancelled string
	Info      string

	// Text, Muted and Faint are the three inks set on the ground.
	Text  string
	Muted string
	Faint string

	// Cyan and Violet extend the non-severity scale: cyan for work in progress,
	// violet for the evidence side of a session. Neither implies urgency.
	Cyan   string
	Violet string
	// High is the step above Warning on the severity ramp.
	High string

	// InfoBg and CriticalBg are the two filled fields the interface paints.
	// Nothing else earns a saturated background.
	InfoBg     string
	CriticalBg string

	// PillText is fixed, not themeable. See BaseTheme.
	PillText string
}

// surfaceLadder is the order the ground slots are asserted in. Keeping the order
// in one place means the contrast test and the theme cannot disagree about which
// surface is meant to be in front of which.
var surfaceLadder = [...]string{"Inset", "Base", "Surface", "Raised"}

// resolve layers a tool palette over the base.
//
// Every slot falls back independently, so a theme setting one colour is valid
// and complete. This is the mandatory fallback: a tool with no theme, or a
// theme that sets nothing, resolves to exactly the base palette.
func (p Palette) resolve() Palette {
	out := BaseTheme
	for _, f := range p.slots(&out) {
		if *f.src != "" {
			*f.dst = *f.src
		}
	}
	return out
}

// slots pairs every field of two palettes so resolve and the depth tests can walk
// them together. Hand-listing the slots once is what stops a new slot being
// added to the palette and forgotten in resolve, which would silently leave
// every tool theme's version of it empty.
func (p Palette) slots(dst *Palette) []struct{ dst, src *string } {
	return []struct{ dst, src *string }{
		{&dst.Inset, &p.Inset}, {&dst.Base, &p.Base},
		{&dst.Surface, &p.Surface}, {&dst.Raised, &p.Raised},
		{&dst.Rule, &p.Rule}, {&dst.Border, &p.Border},
		{&dst.Accent, &p.Accent}, {&dst.AccentHi, &p.AccentHi},
		{&dst.AccentLo, &p.AccentLo}, {&dst.AccentBg, &p.AccentBg},
		{&dst.Warning, &p.Warning}, {&dst.Danger, &p.Danger},
		{&dst.Cancelled, &p.Cancelled}, {&dst.Info, &p.Info},
		{&dst.Text, &p.Text}, {&dst.Muted, &p.Muted}, {&dst.Faint, &p.Faint},
		{&dst.Cyan, &p.Cyan}, {&dst.Violet, &p.Violet}, {&dst.High, &p.High},
		{&dst.InfoBg, &p.InfoBg}, {&dst.CriticalBg, &p.CriticalBg},
		{&dst.PillText, &p.PillText},
	}
}

// at returns the palette as a terminal of this depth will actually show it.
//
// Above 256 colours the palette is returned unchanged. Below it every slot is
// replaced by an explicit index, because this is the one calculation the
// interface refuses to delegate. Nearest-colour is a Euclidean distance, and a
// distance has no idea that a divider has to stay a divider: measured against
// this ground, lipgloss's own conversion turns Rule into a bright green, turns
// the CRITICAL background into black, and collapses four distinct surfaces into
// one.
func (p Palette) at(d ColorDepth) Palette {
	switch d {
	case DepthNone:
		// The terminal renders no colour at all, so which colour a slot holds
		// cannot reach the screen. The hexes are kept rather than replaced so a
		// theme built in this state still reports a meaningful palette to a
		// component asking about it.
		return p
	case Depth16:
		return p.at256().at16()
	case DepthTrueColor:
		return p
	}
	return p.at256()
}

// at256 replaces every slot with an explicit 256-colour index.
func (p Palette) at256() Palette {
	out := p
	// Surfaces become a monotonic grey ramp. Grey rather than green-black
	// because the 256-colour cube has no dark greens to find: the nearest cube
	// entries to these grounds are greys anyway, and asking for greys says so.
	// The ramp stays monotonic here even though it cannot keep the 1.15 steps:
	// this ramp advances ten levels at a time, and near black that is less than
	// a step. Ordering is the property that has to survive, and it does.
	out.Inset = color256(232)
	out.Base = color256(233)
	out.Surface = color256(234)
	out.Raised = color256(236)
	// Structure becomes grey too, and Rule sits above its nearest colour so a
	// divider reads as a divider rather than as a suggestion of one. Rule takes
	// 238 and Border 239: the ramp only advances ten levels at a time here, and
	// 240 is already above the band once it is measured against Inset.
	out.Rule = color256(238)
	out.Border = color256(239)

	out.Text = color256(195)
	out.Muted = color256(109)
	// Faint is a foreground ink on the lightest ground, so it has to be a grey
	// above index 100 rather than the dark grey a nearest-colour search finds.
	out.Faint = color256(102)
	out.Accent = color256(42)
	out.AccentHi = color256(85)
	out.AccentLo = color256(35)

	out.Info = color256(75)
	out.Cyan = color256(80)
	out.Violet = color256(141)
	out.Warning = color256(221)
	out.High = color256(209)
	// Danger is pink rather than the red-orange of High, and lands a full step
	// darker so the two severities stay apart by hue and by weight.
	out.Danger = color256(204)
	// Cancelled takes the dim end of cyan for the same reason it does at sixteen
	// colours. The cube has no desaturated blue-grey above the contrast bar, so
	// this is the only free hue in the neighbourhood, and matching the
	// sixteen-colour choice means a terminal that drops depth keeps its meaning.
	out.Cancelled = color256(37)
	out.CriticalBg = color256(160)

	// The two soft fills go, because the 256-colour cube has no dark green dark
	// enough to sit under #22E58B and stay a field rather than a hole. The
	// cube's dark greens are pure #00AF5F and darker, and an accent on one of
	// those is a contrast failure, not a pill. So at this depth a pill is bold
	// coloured text on the ground, which is the same information carried by
	// weight instead of fill. See pillStyle.
	out.AccentBg, out.InfoBg = "", ""
	return out
}

// at16 maps the palette onto the sixteen base colours.
//
// The mapping is explicit rather than computed for two reasons. Sixteen colours
// are few enough that two slots collide if they are merely similar, and a
// collision means a severity and a status render identically. And the ramp is
// not one scale: 0-7 are dim and 8-15 are bright, so 37 and 97 are a long step
// apart while 32 and 92 are a long step apart, and text needs the bright half.
//
// No two meaning-bearing slots share an index, which
// TestPaletteSixteenColoursAreDistinct asserts. Rule, Border and Faint do share
// one: a border and a rule are structure rather than meaning, and spending a
// sixteenth colour on one would cost a severity.
func (p Palette) at16() Palette {
	out := p
	out.Text = color16(97)
	out.Muted = color16(37)
	out.Faint = color16(90)
	out.Accent = color16(92)
	// AccentHi is Accent in bold. The bright half of a sixteen-colour terminal
	// has no green left in it, so emphasis has to come from weight.
	out.AccentHi = color16(92)
	out.AccentLo = color16(32)
	out.Info = color16(94)
	out.Cyan = color16(96)
	out.Violet = color16(95)
	out.Warning = color16(93)
	out.High = color16(33)
	out.Danger = color16(91)
	// Cancelled takes the dim half of cyan, the same long step Muted takes from
	// Text. A drained, quiet word should look like the accent turned down rather
	// than like another alert, and reusing a bright hue would make it one.
	out.Cancelled = color16(36)
	out.Rule = color16(90)
	out.Border = color16(90)
	out.PillText = color16(97)

	// There is no field to paint at this depth, so the ground and the two soft
	// backgrounds are emptied rather than approximated. An empty slot means
	// "paint no fill", and the styles built from it become the identity
	// transform.
	out.Inset, out.Base, out.Surface, out.Raised = "", "", "", ""
	out.AccentBg, out.InfoBg = "", ""
	// CRITICAL is the exception: it is the one badge that must read as loudest,
	// so it keeps a field and takes the dim half of the ramp, which is the only
	// red available. pillStyle is the one place that lets a fill through at this
	// depth, and only for this slot.
	out.CriticalBg = color16(41)
	return out
}

// sgrPrefix marks a palette slot that holds a bare SGR colour parameter rather
// than a hex.
const sgrPrefix = "sgr:"

func color256(idx uint8) string { return hexOf(Color256(idx).RGB()) }

// color16 records an explicit SGR colour parameter for a sixteen-colour
// terminal.
//
// The parameter is stored rather than the RGB it resolves to because the two are
// not interchangeable on the wire. Palette entry 15 is #FFFFFF and SGR 97 is
// #FFFFFF, but a sixteen-colour terminal is written to with 97, and asking
// lipgloss for entry 15 makes it emit a 256-colour escape the terminal cannot
// read. Storing the parameter is what keeps the rendered bytes honest.
func color16(sgr uint8) string { return sgrPrefix + strconv.Itoa(int(sgr)) }

// ansi16Index converts an SGR colour parameter to a palette index. 30-37 are the
// dim half and 90-97 the bright half, and the palette numbers them 0-7 and 8-15.
func ansi16Index(sgr uint8) (uint, bool) {
	switch {
	case sgr >= 30 && sgr <= 37:
		return uint(sgr - 30), true
	case sgr >= 90 && sgr <= 97:
		return uint(sgr-90) + 8, true
	case sgr >= 40 && sgr <= 47:
		return uint(sgr - 40), true
	case sgr >= 100 && sgr <= 107:
		return uint(sgr-100) + 8, true
	}
	return 0, false
}

// paletteColor turns a palette slot value into something lipgloss will render.
//
// A slot holds a hex everywhere a hex can be shown and an SGR parameter at
// sixteen colours, so this is where the two forms meet.
func paletteColor(slot string) lipgloss.TerminalColor {
	if sgr, ok := strings.CutPrefix(slot, sgrPrefix); ok {
		if n, err := strconv.ParseUint(sgr, 10, 8); err == nil {
			if idx, ok := ansi16Index(uint8(n)); ok {
				return lipgloss.ANSIColor(idx)
			}
		}
		return lipgloss.NoColor{}
	}
	return lipgloss.Color(slot)
}

// slotRGB reads a palette slot back as a colour the contrast tests can measure.
// It understands both forms, so a test can measure the palette that will
// actually be rendered rather than the one that was written down.
func slotRGB(slot string) (RGB, bool) {
	if sgr, ok := strings.CutPrefix(slot, sgrPrefix); ok {
		n, err := strconv.ParseUint(sgr, 10, 8)
		if err != nil {
			return RGB{}, false
		}
		idx, ok := ansi16Index(uint8(n))
		if !ok {
			return RGB{}, false
		}
		return Color16(uint8(idx)).RGB(), true
	}
	return parseHex(slot)
}

func hexOf(v RGB) string { return fmt.Sprintf("#%02X%02X%02X", v.R, v.G, v.B) }

// parseHex reads a #RRGGBB string back into a colour. It exists so the contrast
// tests can measure the palette that will actually be rendered rather than the
// one that was written down.
func parseHex(hex string) (RGB, bool) {
	h := strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(h) != 6 {
		return RGB{}, false
	}
	var v RGB
	out := []*uint8{&v.R, &v.G, &v.B}
	for i, o := range out {
		n, ok := parseByte(h[i*2], h[i*2+1])
		if !ok {
			return RGB{}, false
		}
		*o = n
	}
	return v, true
}

func parseByte(hi, lo byte) (uint8, bool) {
	h, ok1 := hexDigit(hi)
	l, ok2 := hexDigit(lo)
	if !ok1 || !ok2 {
		return 0, false
	}
	return h<<4 | l, true
}

func hexDigit(c byte) (uint8, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// Theme carries the styles used across the interface.
//
// Colour is decided once, at startup, from NO_COLOR and the detected terminal.
// Every style is a no-op when colour is off, so there is one rendering path
// rather than a plain-text variant that could drift from it.
//
// A Theme is built from a Palette, and a Palette is built from BaseTheme plus
// whatever a tool supplied. No rendering component holds a colour: components
// ask the theme. That is what keeps branding out of the presentation code -- a
// component that hard-coded a hex would be a tool's identity leaking into the
// shared layer, and a second place to change when the palette does.
type Theme struct {
	// Color reports whether styling is active. When false every style is the
	// identity transform and NO_COLOR is honoured on the one rendering path.
	Color bool
	// Depth is the colour depth the styles were resolved for. Components need it
	// for decisions colour cannot express, such as whether a framed panel is
	// drawn at all.
	Depth ColorDepth

	// Palette is the resolved palette, kept for diagnostics and for components
	// that need a raw colour rather than a style.
	Palette Palette

	// Identity.
	Title   lipglossStyle
	Version lipglossStyle
	Rule    lipglossStyle

	// Structure. These carry the shape of the interface rather than the state
	// of the work: the gutter that groups a command with its output, and the
	// heading, count and column styles that separate a section label from the
	// data beneath it. They are deliberately the quietest slots in the theme,
	// because structure is read constantly and must never compete for
	// attention with something that actually happened.
	Gutter  lipglossStyle
	Heading lipglossStyle
	Count   lipglossStyle
	Column  lipglossStyle

	// Status. These are the only saturated colours in the resting interface.
	Ready     lipglossStyle
	Running   lipglossStyle
	Failed    lipglossStyle
	Cancelled lipglossStyle
	Warning   lipglossStyle

	// Session.
	Command lipglossStyle
	// Output is the tool's own printed text. It is deliberately unadorned: the
	// interface did not interpret it, so it does not dress it up.
	Output  lipglossStyle
	Arg     lipglossStyle
	Group   lipglossStyle
	Label   lipglossStyle
	Value   lipglossStyle
	Detail  lipglossStyle
	Success lipglossStyle

	// Composer.
	Prompt lipglossStyle
	Hint   lipglossStyle
	Badge  lipglossStyle
	// ModeBar is the coloured strip that carries the composer's mode, so the
	// session's state is legible without reading the status word in the header.
	ModeBar lipglossStyle
	ModeFg  lipglossStyle
	// Tip is the rotating one-line hint. It is the quietest text in the
	// interface on purpose: it is the first thing to disappear.
	Tip lipglossStyle

	// Severity.
	Critical lipglossStyle
	High     lipglossStyle
	Medium   lipglossStyle
	Low      lipglossStyle
	Info     lipglossStyle
	// Cyan and Violet are the two non-urgency hues, for work in progress and for
	// evidence respectively.
	Cyan   lipglossStyle
	Violet lipglossStyle

	// Progress.
	BarFill  lipglossStyle
	BarEmpty lipglossStyle
	BarLead  lipglossStyle

	// Ground and surfaces. These are the painted fills, in the order they stack.
	Base    lipglossStyle
	Inset   lipglossStyle
	Surface lipglossStyle
	Raised  lipglossStyle

	// Border draws a panel edge; FocusRing is the same edge on the panel that
	// has keyboard focus. They are separate slots because the difference is the
	// only cue that says where a keystroke will land.
	Border    lipglossStyle
	FocusRing lipglossStyle
	Selection lipglossStyle
	// Activity severity, for the region that renders live events.
	Activity lipglossStyle

	// Pills. A pill is a filled label; the caps are drawn in the pill's own
	// background so the field reaches the edge of the cell.
	Pill         lipglossStyle
	PillCritical lipglossStyle
	PillHigh     lipglossStyle
	PillMuted    lipglossStyle
}

// newTheme builds a theme from a palette, dropping all styling when colour is
// disabled.
//
// The palette argument is the already-resolved result of layering a tool's
// theme over BaseTheme, so this function has no knowledge of where the colours
// came from. A nil palette is the base.
func newTheme(color bool, p *Palette) Theme {
	// Depth is resolved even when colour is off, because the palette a terminal
	// can render is not the palette that was written down, and a component still
	// asks the theme how deep it is when deciding whether to draw a frame at all.
	return newThemeAt(color, p, detectColorDepth())
}

// newThemeAt builds a theme for a known depth.
//
// The split exists so the palette resolution can be tested without a terminal.
// Everything above this point is deciding which depth to use; everything below
// is the same work for whatever answer that produced, and a test needs to be able
// to ask the question at each depth in turn.
func newThemeAt(color bool, p *Palette, depth ColorDepth) Theme {
	if noColorRequested() {
		color = false
	}
	var pal Palette
	if p != nil {
		pal = p.resolve()
	} else {
		pal = BaseTheme.resolve()
	}
	if !color {
		return plainTheme(pal, depth)
	}
	pal = pal.at(depth)

	return Theme{
		Color:   true,
		Depth:   depth,
		Palette: pal,

		Title:   bold(pal.Accent),
		Version: fg(pal.Faint),
		Rule:    fg(pal.Rule),

		// Structure stays close to the ground on purpose. The gutter is one step
		// above the background, so a command's body is visibly attached to its
		// command without the attachment being the first thing the eye lands on.
		Gutter:  fg(pal.Rule),
		Heading: bold(pal.Muted),
		Count:   fg(pal.Faint),
		Column:  fg(pal.Faint),

		Ready:     fg(pal.Accent),
		Running:   fg(pal.Accent),
		Failed:    fg(pal.Danger),
		Cancelled: fg(pal.Cancelled),
		Warning:   fg(pal.Warning),

		Command: bold(pal.Text),
		Arg:     fg(pal.Accent),
		Group:   fg(pal.Muted),
		Label:   fg(pal.Muted),
		Value:   fg(pal.Text),
		Output:  fg(pal.Text),
		Detail:  fg(pal.Faint),
		Success: fg(pal.Accent),

		Prompt: bold(pal.Accent),
		Hint:   fg(pal.Faint),
		Badge:  badgeStyle(pal.Info, pal.Surface),
		// The mode bar is a field with ink on it, not accent on accent. Accent on
		// Accent is invisible, and the mode bar exists precisely so the current
		// input target is legible without reading the hint.
		ModeBar: fill(pal.AccentBg),
		ModeFg:  fg(pal.AccentHi),
		Tip:     fg(pal.Faint),

		Critical: fg(pal.Danger),
		High:     fg(pal.High),
		Medium:   fg(pal.Warning),
		Low:      fg(pal.Info),
		Info:     fg(pal.Info),
		Cyan:     fg(pal.Cyan),
		Violet:   fg(pal.Violet),

		BarFill:  fg(pal.Accent),
		BarEmpty: fg(pal.Rule),
		BarLead:  fg(pal.AccentHi),

		Base:    fill(pal.Base),
		Inset:   fill(pal.Inset),
		Surface: fill(pal.Surface),
		Raised:  fill(pal.Raised),

		Border:    fg(pal.Border),
		FocusRing: bold(pal.Accent),
		Selection: bold(pal.Accent),
		Activity:  fg(pal.Muted),

		Pill:         pillStyle(depth, pal.Text, pal.AccentBg),
		PillCritical: criticalPillStyle(pal.PillText, pal.CriticalBg),
		PillHigh:     pillStyle(depth, pal.Warning, ""),
		PillMuted:    pillStyle(depth, pal.Muted, ""),
	}
}

// plainTheme is the no-colour theme. Every style is the identity transform, so
// one rendering path serves both cases and a plain-text variant cannot drift.
func plainTheme(pal Palette, depth ColorDepth) Theme {
	plain := newPlainStyle()
	t := Theme{Color: false, Depth: depth, Palette: pal}
	for _, s := range []*lipglossStyle{
		&t.Title, &t.Version, &t.Rule, &t.Gutter, &t.Heading, &t.Count, &t.Column,
		&t.Ready, &t.Running, &t.Failed, &t.Cancelled, &t.Warning,
		&t.Command, &t.Output, &t.Arg, &t.Group, &t.Label, &t.Value, &t.Detail,
		&t.Success, &t.Prompt, &t.Hint, &t.Badge, &t.ModeBar, &t.ModeFg, &t.Tip,
		&t.Critical, &t.High, &t.Medium, &t.Low, &t.Info, &t.Cyan, &t.Violet,
		&t.BarFill, &t.BarEmpty, &t.BarLead,
		&t.Base, &t.Inset, &t.Surface, &t.Raised,
		&t.Border, &t.FocusRing, &t.Selection, &t.Activity,
		&t.Pill, &t.PillCritical, &t.PillHigh, &t.PillMuted,
	} {
		*s = plain
	}
	return t
}

// newPlainStyle is the identity transform, used for every slot when colour is
// off.
func newPlainStyle() lipglossStyle { return newPlainStyleImpl() }

// colorEnabled decides whether to emit colour.
//
// NO_COLOR wins over everything, including a TERM that promises colour: setting
// it is a request for plain output, and a library overriding that would break
// whatever the caller is piping into.
func colorEnabled(out *osFile) bool {
	if noColorRequested() {
		return false
	}
	if getenv("TERM") == "dumb" {
		return false
	}
	// A stream that is not a terminal cannot render styling meaningfully, and
	// emitting escape codes into a pipe is a defect rather than a nicety.
	return isTerminal(out)
}

// noColorRequested reports whether NO_COLOR is set.
//
// The variable is presence-based, per the convention: `NO_COLOR=` and
// `NO_COLOR=0` are both requests for plain output, and only a shell that has
// unset it entirely means colour. Reading the value would honour a
// user who wrote `NO_COLOR=0` meaning "off", which is precisely the request
// the standard's presence rule exists to make unambiguous.
func noColorRequested() bool {
	_, ok := lookupEnv("NO_COLOR")
	return ok
}

// severityTag renders a severity as a fixed-width uppercase word.
//
// The full word rather than an abbreviation: "CRITICAL" and "HIGH" are the two
// labels an operator triages on, and shortening them to fit a column trades the
// one thing the label exists to say for a few characters of alignment. The
// width is fixed instead, which is what actually produces the column.
func (t Theme) severityTag(sev string) string {
	return pad(strings.ToUpper(severityWord(sev)), severityColWidth)
}

// severityWord is the canonical name for a severity label, folding the spelling
// variants tools emit into one word.
func severityWord(sev string) string {
	switch strings.ToUpper(strings.TrimSpace(sev)) {
	case "CRITICAL", "CRIT":
		return "CRITICAL"
	case "HIGH":
		return "HIGH"
	case "MEDIUM", "MODERATE", "MED", "MOD":
		return "MEDIUM"
	case "LOW":
		return "LOW"
	case "INFO", "INFORMATIONAL", "NOTE":
		return "INFO"
	default:
		return "INFO"
	}
}

// severityRank orders severities for tallying and sorting. An unrecognised
// label ranks below LOW rather than above it: inventing a severity is worse
// than understating one.
func severityRank(sev string) int {
	switch severityWord(sev) {
	case "CRITICAL":
		return 4
	case "HIGH":
		return 3
	case "MEDIUM":
		return 2
	case "LOW":
		return 1
	default:
		return 0
	}
}

// styleForSeverityWord returns the style for a canonical severity word.
func (t Theme) styleForSeverityWord(word string) lipglossStyle {
	switch word {
	case "CRITICAL":
		return t.Critical
	case "HIGH":
		return t.High
	case "MEDIUM":
		return t.Medium
	case "LOW":
		return t.Low
	default:
		return t.Info
	}
}

// pillFor returns the filled style for a severity word. CRITICAL is the only
// severity that gets a filled pill with white ink, because it is the only label
// that has to win a scan against everything else on screen.
func (t Theme) pillFor(word string) lipglossStyle {
	switch word {
	case "CRITICAL":
		return t.PillCritical
	case "HIGH":
		return t.PillHigh
	default:
		return t.PillMuted
	}
}

// worstSeverity returns the most severe label present in a set of findings, or
// an empty string when there are none. It is what lets a summary line report
// "4 findings" in the colour of the worst of them.
func worstSeverity(fs []Finding) string {
	worst, rank := "", -1
	for _, f := range fs {
		if r := severityRank(f.Severity); r > rank {
			rank, worst = r, severityWord(f.Severity)
		}
	}
	if rank <= 0 {
		return ""
	}
	return worst
}

// statusStyle maps an execution status onto its style.
func (t Theme) statusStyle(s Status) lipglossStyle {
	switch s {
	case StatusRunning:
		return t.Running
	case StatusFailed:
		return t.Failed
	case StatusCancelled:
		return t.Cancelled
	default:
		return t.Success
	}
}

// levelStyle maps an event level onto a style.
//
// A level the TUI has not seen falls back to muted rather than to a colour
// that implies a severity it does not have. Guessing severity from an unknown
// label is how a warning ends up rendered as a failure.
func (t Theme) levelStyle(level string) lipglossStyle {
	switch levelSeverity(level) {
	case 0, 1:
		return t.Failed
	case 2:
		return t.Warning
	case 3:
		return t.Info
	default:
		return t.Detail
	}
}
