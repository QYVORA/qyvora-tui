package tui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestSurfaceLadderIsMonotonic(t *testing.T) {
	pal := BaseTheme.resolve().at(DepthTrueColor)
	var prevL float64 = -1.0
	for i, n := range surfaceLadder {
		v := slotOfT(pal, n)
		if v == "" {
			t.Fatalf("%s is empty at truecolor", n)
		}
		c, ok := slotRGB(v)
		if !ok {
			t.Fatalf("%s has bad hex %q", n, v)
		}
		l := c.luminance()
		if i == 0 {
			prevL = l
			continue
		}
		r := (l + 0.05) / (prevL + 0.05)
		if r < 1.15 {
			t.Fatalf("surface ladder step %s->%s is %.4f < 1.15 (L %.5f vs %.5f)", surfaceLadder[i-1], n, r, prevL, l)
		}
		if l <= prevL {
			t.Fatalf("surface %s not increasing: %.5f <= %.5f", n, l, prevL)
		}
		prevL = l
	}
}

func TestPaletteHasNoSemanticIndexCollisionsAt16(t *testing.T) {
	pal := BaseTheme.resolve().at(Depth16)
	seen := map[string]string{}
	for _, s := range []struct {
		key  string
		slot string
	}{
		{"Text", pal.Text},
		{"Muted", pal.Muted},
		{"Faint", pal.Faint},
		{"Accent", pal.Accent},
		{"AccentLo", pal.AccentLo},
		{"Info", pal.Info},
		{"Cyan", pal.Cyan},
		{"Violet", pal.Violet},
		{"Warning", pal.Warning},
		{"High", pal.High},
		{"Danger", pal.Danger},
		{"Cancelled", pal.Cancelled},
	} {
		if s.slot == "" {
			continue
		}
		if other, ok := seen[s.slot]; ok && other != s.key {
			t.Fatalf("%s and %s share %q at 16-colour", other, s.key, s.slot)
		}
		seen[s.slot] = s.key
	}
}

func TestCriticalColorsAreWhiteOnRedAtAllDepths(t *testing.T) {
	for _, d := range []ColorDepth{DepthTrueColor, Depth256, Depth16} {
		pal := BaseTheme.resolve().at(d)
		cb := pal.CriticalBg
		pt := pal.PillText
		if cb == "" {
			t.Fatalf("CriticalBg empty at %s", d)
		}
		if pt == "" {
			t.Fatalf("PillText empty at %s", d)
		}
		cbg, ok := slotRGB(cb)
		if !ok {
			t.Fatalf("bad CriticalBg %q at %s", cb, d)
		}
		cpt, ok := slotRGB(pt)
		if !ok {
			t.Fatalf("bad PillText %q at %s", pt, d)
		}
		if cpt != (RGB{0xFF, 0xFF, 0xFF}) {
			t.Fatalf("PillText not #FFFFFF at %s: %v", d, cpt)
		}
		if contrast(cbg, cpt) < 4.5 {
			t.Fatalf("CRITICAL contrast %.2f < 4.5 at %s", contrast(cbg, cpt), d)
		}
	}
}

func TestSixteenColorPillCriticalHasFill(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI)
	th := newTheme(true, nil)
	th.Depth = Depth16
	p := th.PillCritical.Render("CRITICAL")
	if !strings.Contains(p, "\x1b[") {
		t.Fatalf("expected SGR in 16-color critical pill: %q", p)
	}
	if !strings.Contains(p, "41m") && !strings.Contains(p, ";41m") {
		t.Fatalf("expected 16-color red background 41 in %q", p)
	}
}
func slotOfT(p Palette, n string) string {
	switch n {
	case "Inset":
		return p.Inset
	case "Base":
		return p.Base
	case "Surface":
		return p.Surface
	case "Raised":
		return p.Raised
	case "Rule":
		return p.Rule
	case "Border":
		return p.Border
	case "Text":
		return p.Text
	case "Muted":
		return p.Muted
	case "Faint":
		return p.Faint
	case "Accent":
		return p.Accent
	case "AccentHi":
		return p.AccentHi
	case "AccentLo":
		return p.AccentLo
	case "AccentBg":
		return p.AccentBg
	case "Info":
		return p.Info
	case "InfoBg":
		return p.InfoBg
	case "Cyan":
		return p.Cyan
	case "Violet":
		return p.Violet
	case "Warning":
		return p.Warning
	case "High":
		return p.High
	case "Danger":
		return p.Danger
	case "Cancelled":
		return p.Cancelled
	case "CriticalBg":
		return p.CriticalBg
	case "PillText":
		return p.PillText
	}
	return ""
}

// sgr parses an escape sequence back into what it says on the wire.
//
// The palette tests are only worth anything if they measure the bytes the
// terminal receives rather than the hex someone typed. Emitting a style and
// reading the sequence back is the only way to catch a mapping that is correct
// as a table and wrong as an escape, which is how a colour ends up asking for a
// 256-colour index on a sixteen-colour terminal.
func sgr(t *testing.T, rendered string) (params string, bold bool) {
	t.Helper()
	i := strings.Index(rendered, "\x1b[")
	if i < 0 {
		return "", false
	}
	rest := rendered[i+2:]
	j := strings.Index(rest, "m")
	if j < 0 {
		return "", false
	}
	params = rest[:j]
	return params, strings.Contains(params, "1")
}

// TestSixteenColorSlotsAreWrittenAsBasicEscapes is the wire-format half of the
// sixteen-colour decision: every slot has to be written as a bare 30-37/90-97
// parameter, never as a 38;5;n or 38;2;r;g;b sequence.
func TestSixteenColorSlotsAreWrittenAsBasicEscapes(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI)
	defer lipgloss.SetColorProfile(termenv.TrueColor)

	pal := BaseTheme.resolve().at(Depth16)
	for _, s := range []struct {
		name string
		slot string
	}{
		{"Text", pal.Text}, {"Muted", pal.Muted}, {"Faint", pal.Faint},
		{"Accent", pal.Accent}, {"AccentLo", pal.AccentLo}, {"Info", pal.Info},
		{"Cyan", pal.Cyan}, {"Violet", pal.Violet}, {"Warning", pal.Warning},
		{"High", pal.High}, {"Danger", pal.Danger}, {"Cancelled", pal.Cancelled},
	} {
		out := fg(s.slot).Render("x")
		params, _ := sgr(t, out)
		if params == "" {
			t.Errorf("%s produced no SGR at 16 colours: %q", s.name, out)
			continue
		}
		if strings.HasPrefix(params, "38;") {
			t.Errorf("%s wrote %q, which is not a basic 16-colour escape", s.name, params)
		}
		if _, err := strconv.Atoi(params); err != nil {
			t.Errorf("%s wrote %q, which is not a single colour parameter", s.name, params)
		}
	}
}

// TestAccentHiIsAccentInBoldAtSixteenColours pins the one deliberate collision
// in the sixteen-colour map. The bright half of a sixteen-colour terminal has no
// green left in it, so AccentHi cannot be a different colour and is instead the
// same colour at full weight.
func TestAccentHiIsAccentInBoldAtSixteenColours(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI)
	defer lipgloss.SetColorProfile(termenv.TrueColor)

	pal := BaseTheme.resolve().at(Depth16)
	if pal.Accent != pal.AccentHi {
		t.Fatalf("Accent %q and AccentHi %q should share an index at 16 colours", pal.Accent, pal.AccentHi)
	}
	params, bold := sgr(t, bold(pal.AccentHi).Render("x"))
	if !bold {
		t.Errorf("AccentHi lost its weight at 16 colours: %q", params)
	}
}

// TestSoftFillsAreGoneAtReducedDepth covers the pill fallbacks. At 256 colours
// AccentBg and InfoBg are emptied because the cube has no dark green dark enough
// to sit under the accent and stay a field; at sixteen colours every fill goes
// except CRITICAL.
func TestSoftFillsAreGoneAtReducedDepth(t *testing.T) {
	for _, d := range []ColorDepth{Depth256, Depth16} {
		pal := BaseTheme.resolve().at(d)
		if pal.AccentBg != "" {
			t.Errorf("AccentBg is %q at %s; a pill there would be ink on a hole", pal.AccentBg, d)
		}
		if pal.InfoBg != "" {
			t.Errorf("InfoBg is %q at %s", pal.InfoBg, d)
		}
		if pal.CriticalBg == "" {
			t.Errorf("CriticalBg is empty at %s, so CRITICAL cannot be the loudest thing", d)
		}
	}
	if pal := BaseTheme.resolve().at(Depth16); pal.Base != "" || pal.Surface != "" {
		t.Errorf("the ground should not be painted at 16 colours: %+v", pal)
	}
}

// TestTextBearingInksClearTheBarOnEveryGround measures the worst surface for
// every ink that carries meaning, at each depth that can render one.
func TestTextBearingInksClearTheBarOnEveryGround(t *testing.T) {
	const faintBar = 3.0
	const textBar = 4.5
	for _, d := range []ColorDepth{DepthTrueColor, Depth256} {
		pal := BaseTheme.resolve().at(d)
		grounds := make([]RGB, 0, len(surfaceLadder))
		for _, g := range surfaceLadder {
			if c, ok := slotRGB(pal.Surface); ok {
				_ = c
			}
			c, ok := slotRGB(slotOfT(pal, g))
			if !ok {
				t.Fatalf("%s is not renderable at %s", g, d)
			}
			grounds = append(grounds, c)
		}
		for _, ink := range []struct {
			name string
			slot string
			bar  float64
		}{
			{"Text", pal.Text, textBar},
			{"Muted", pal.Muted, textBar},
			{"Faint", pal.Faint, faintBar},
			{"Accent", pal.Accent, textBar},
			{"AccentHi", pal.AccentHi, textBar},
			{"AccentLo", pal.AccentLo, textBar},
			{"Info", pal.Info, textBar},
			{"Cyan", pal.Cyan, textBar},
			{"Violet", pal.Violet, textBar},
			{"Warning", pal.Warning, textBar},
			{"High", pal.High, textBar},
			{"Danger", pal.Danger, textBar},
			{"Cancelled", pal.Cancelled, textBar},
		} {
			c, ok := slotRGB(ink.slot)
			if !ok {
				t.Fatalf("%s is not renderable at %s", ink.name, d)
			}
			worst, on := 99.0, ""
			for i, g := range grounds {
				if r := contrast(c, g); r < worst {
					worst, on = r, surfaceLadder[i]
				}
			}
			if worst < ink.bar {
				t.Errorf("%s: %s on %s is %.2f at %s, below %.1f",
					ink.name, ink.slot, on, worst, d, ink.bar)
			}
		}
	}
}

// TestStructureSitsInItsBand keeps Rule and Border visible without making them
// legible. A divider that reads as text is a bug; one that cannot be found is
// also a bug.
func TestStructureSitsInItsBand(t *testing.T) {
	const lo, hi = 1.2, 2.5
	for _, d := range []ColorDepth{DepthTrueColor, Depth256} {
		pal := BaseTheme.resolve().at(d)
		for _, s := range []struct {
			name string
			slot string
		}{{"Rule", pal.Rule}, {"Border", pal.Border}} {
			c, ok := slotRGB(s.slot)
			if !ok {
				t.Fatalf("%s is not renderable at %s", s.name, d)
			}
			for i, g := range surfaceLadder {
				gc, ok := slotRGB(slotOfT(pal, g))
				if !ok {
					continue
				}
				r := contrast(c, gc)
				if r < lo || r > hi {
					t.Errorf("%s on %s at %s is %.2f, outside the %.1f-%.1f band",
						s.name, g, d, r, lo, hi)
				}
				_ = i
			}
		}
	}
}

// TestReducedDepthSurfacesAreGreyAndOrdered is the regression guard for the
// failure this table exists to prevent: lipgloss's nearest-colour search turns
// Rule into a bright green and collapses four surfaces into one.
func TestReducedDepthSurfacesAreGreyAndOrdered(t *testing.T) {
	pal := BaseTheme.resolve().at(Depth256)
	for _, g := range surfaceLadder {
		c, ok := slotRGB(slotOfT(pal, g))
		if !ok {
			t.Fatalf("%s is not renderable at 256 colours", g)
		}
		if c.R != c.G || c.G != c.B {
			t.Errorf("%s is %v at 256 colours; the ramp is grey by construction", g, c)
		}
	}
	for _, s := range []struct {
		name string
		slot string
	}{{"Rule", pal.Rule}, {"Border", pal.Border}} {
		c, ok := slotRGB(s.slot)
		if !ok {
			continue
		}
		if c.R != c.G || c.G != c.B {
			t.Errorf("%s is %v at 256 colours; structure is grey, not green", s.name, c)
		}
	}
	var prev float64 = -1
	for _, g := range surfaceLadder {
		c, _ := slotRGB(slotOfT(pal, g))
		if c.luminance() <= prev {
			t.Fatalf("256-colour surface %s is not above the one below it", g)
		}
		prev = c.luminance()
	}
}
