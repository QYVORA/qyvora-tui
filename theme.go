package tui

import "strings"

// BaseTheme is the QYVORA palette every tool inherits unless it supplies its
// own.
//
// The accent is the QYVORA green. Everything else is a near-black ground with
// muted greys, so colour is reserved for the two things worth noticing: the
// prompt, and the state of the work. A terminal agent spends most of its life
// showing text, and colouring all of it is how a session becomes unreadable.
//
// Readability here is not a default to be overridden. A tool theme shifts hue,
// not the amount of colour on screen; a palette that failed contrast against
// this ground would make the interface harder to read, which is the opposite of
// what identity is for.
var BaseTheme = Palette{
	Accent:    "#06B66F", // QYVORA green: prompt, rules, success
	Warning:   "#D9A441",
	Danger:    "#E5484D",
	Cancelled: "#8B8FA3",
	Info:      "#6E7B8B",
	Text:      "#E6E8EB",
	Muted:     "#7A8290",
	Faint:     "#4A5058",
	Rule:      "#262B33",
	Surface:   "#1A1E24",
}

// Palette is the semantic colour set a theme is written in.
//
// A tool fills in only the slots it means to change; everything else falls back
// to the base. That is what makes a partial theme legitimate: a tool with one
// recognisable brand colour should be able to set Accent and inherit the rest,
// rather than restating nine hex values to get one of them right.
type Palette struct {
	// Accent is the tool's identity colour: prompt, rules, success.
	Accent string
	// Warning, Danger, Cancelled and Info carry execution state. These are
	// deliberately NOT part of the branding surface: a tool overriding its own
	// failure colour would make a failure stop reading as a failure.
	Warning   string
	Danger    string
	Cancelled string
	Info      string

	// Text, Muted, Faint and Rule are the ground.
	Text    string
	Muted   string
	Faint   string
	Rule    string
	Surface string
}

// resolve layers a tool palette over the base.
//
// Every slot falls back independently, so a theme setting one colour is valid
// and complete. This is the mandatory fallback: a tool with no theme, or a
// theme that sets nothing, resolves to exactly the base palette.
func (p Palette) resolve() Palette {
	out := BaseTheme
	if p.Accent != "" {
		out.Accent = p.Accent
	}
	// Status colours are only taken from a tool palette when it is clearly
	// supplying a full theme. Overriding them piecemeal would let a tool
	// recolour a failure without meaning to.
	if p.Warning != "" {
		out.Warning = p.Warning
	}
	if p.Danger != "" {
		out.Danger = p.Danger
	}
	if p.Cancelled != "" {
		out.Cancelled = p.Cancelled
	}
	if p.Info != "" {
		out.Info = p.Info
	}
	if p.Text != "" {
		out.Text = p.Text
	}
	if p.Muted != "" {
		out.Muted = p.Muted
	}
	if p.Faint != "" {
		out.Faint = p.Faint
	}
	if p.Rule != "" {
		out.Rule = p.Rule
	}
	if p.Surface != "" {
		out.Surface = p.Surface
	}
	return out
}

// Theme carries the styles used across the interface.
//
// Colour is decided once, at startup, from NO_COLOR and the detected terminal.
// Every style is a no-op when colour is off, so there is one rendering path
// rather than a plain-text variant that could drift from it.
//
// A Theme is built from a Palette, and a Palette is built from QYVORA_BASE plus
// whatever a tool supplied. No rendering component holds a colour: components
// ask the theme. That is what keeps branding out of the presentation code -- a
// component that hard-coded a hex would be a tool's identity leaking into the
// shared layer, and a second place to change when the palette does.
type Theme struct {
	// Color reports whether styling is active. When false every style is the
	// identity transform and NO_COLOR is honoured on the one rendering path.
	Color bool

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

	// Severity.
	Critical lipglossStyle
	High     lipglossStyle
	Medium   lipglossStyle
	Low      lipglossStyle
	Info     lipglossStyle

	// Progress.
	BarFill  lipglossStyle
	BarEmpty lipglossStyle

	// Regions. These are what the adaptive shell draws with, kept here so a
	// layout change never reaches for a colour of its own.
	Surface   lipglossStyle
	Border    lipglossStyle
	Selection lipglossStyle
	// Activity severity, for the region that renders live events.
	Activity lipglossStyle
}

// newTheme builds a theme from a palette, dropping all styling when colour is
// disabled.
//
// The palette argument is the already-resolved result of layering a tool's
// theme over QYVORA_BASE, so this function has no knowledge of where the
// colours came from. A nil palette is the base.
func newTheme(color bool, p *Palette) Theme {
	// NO_COLOR is checked here rather than by each caller, so there is exactly
	// one place that decides whether colour is emitted. A caller that forgets
	// to check is the failure mode this removes: a tool's palette reaching the
	// terminal of someone who asked for no colour at all.
	if noColorRequested() {
		color = false
	}

	// A nil palette is the normal case: most tools have no brand colour worth
	// inventing, and inheriting the base is the correct answer, not a missing
	// feature. resolve() layers whatever was supplied over the base.
	var pal Palette
	if p != nil {
		pal = p.resolve()
	} else {
		pal = BaseTheme
	}

	if !color {
		// With no colour each style is the identity transform, so every styled
		// string renders as its plain text and the interface stays legible
		// without a second implementation.
		plain := newPlainStyle()
		return Theme{
			Color: false, Palette: pal, Title: plain, Version: plain, Rule: plain,
			Ready: plain, Running: plain, Failed: plain, Cancelled: plain,
			Warning: plain,
			Command: plain, Arg: plain, Group: plain, Label: plain,
			Value: plain, Detail: plain, Success: plain, Prompt: plain, Output: plain,
			Hint: plain, Badge: plain, Critical: plain, High: plain,
			Medium: plain, Low: plain, Info: plain, BarFill: plain,
			BarEmpty: plain, Surface: plain, Border: plain, Selection: plain,
			Activity: plain,
			Gutter:   plain, Heading: plain, Count: plain, Column: plain,
		}
	}

	return Theme{
		Color:   true,
		Palette: pal,
		Title:   bold(pal.Accent),
		Version: fg(pal.Faint),
		Rule:    fg(pal.Rule),

		// Structure stays close to the ground on purpose. The gutter is the
		// rule colour, one step above the background, so a command's body is
		// visibly attached to its command without the attachment being the
		// first thing the eye lands on.
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

		Critical: fg(pal.Danger),
		High:     fg(pal.Warning),
		Medium:   fg(pal.Warning),
		Low:      fg(pal.Info),
		Info:     fg(pal.Muted),

		BarFill:  fg(pal.Accent),
		BarEmpty: fg(pal.Rule),

		Surface:   bg(pal.Surface),
		Border:    fg(pal.Rule),
		Selection: bold(pal.Accent),
		Activity:  fg(pal.Muted),
	}
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
	case "MEDIUM", "MODERATE", "MED":
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
