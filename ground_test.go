package tui

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestGroundRestoresAfterEveryReset(t *testing.T) {
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	lipgloss.SetColorProfile(termenv.TrueColor)
	th := newTheme(true, nil)

	// One styled span in the middle of a row is the ordinary case: a status
	// pill, a highlighted severity, a dimmed version.
	line := "start " + th.Surface.Render("PILL") + " end"
	got := ground(th.Base, line, 30)

	// "start PILL end" is 14 columns, so a 30-column row carries 16 of padding.
	if want, have := "start PILL end"+strings.Repeat(" ", 16), stripANSI(got); want != have {
		t.Fatalf("text changed:\n got %q\nwant %q", have, want)
	}
	if w := lipgloss.Width(stripANSI(got)); w != 30 {
		t.Errorf("width %d, want 30", w)
	}

	// The whole point. The base background must be re-established after the
	// span's reset, or the tail of the row falls back to the terminal's own
	// background and the row is striped.
	base := leadingSGR(th.Base.Render("x"))
	resets := strings.Count(got, "\x1b[0m")
	emits := strings.Count(got, base)
	if emits < resets+1 {
		t.Errorf("base emitted %d times for %d resets and a row start: the ground "+
			"is lost after a styled span\n got %q", emits, resets, got)
	}
}

func TestGroundKeepsForeignAndMalformedSequences(t *testing.T) {
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	lipgloss.SetColorProfile(termenv.TrueColor)
	th := newTheme(true, nil)

	const width = 24
	base := leadingSGR(th.Base.Render("x"))
	cases := []struct {
		name      string
		line      string
		malformed bool
	}{
		// Well-formed input: the row is filled to the terminal's edge, so the
		// visible result is the content plus padding to width.
		{name: "tool output with its own escapes", line: "\x1b[38;2;255;0;0mred\x1b[0m tail"},
		{name: "bracketed paste", line: "\x1b[200~typed\x1b[201~ rest"},
		{name: "osc title set", line: "\x1b]0;title\x07 rest"},
		{name: "reset only", line: "\x1b[0m tail"},
		// Malformed input: an unterminated CSI is not matched by ansiCode on
		// purpose, because a partial match can leave a bare ESC for a wrapper to
		// slice through. lipgloss.Width then reads the tail as escape payload,
		// so visible width is not measurable and only the bytes are checked.
		{name: "unterminated escape", line: "before\x1b[38;2;255;0;0", malformed: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ground(th.Base, c.line, width)

			// Nothing may be dropped or reordered. This is still the operator's
			// output, and the ground changes a background, not content. The
			// comparison is against the stripped input, because stripANSI
			// legitimately consumes an OSC and leaves no text behind for it.
			if have, want := stripANSI(got), padTo(stripANSI(c.line), width); !c.malformed {
				if want != have {
					t.Errorf("content changed:\n got %q\nwant %q", have, want)
				}
				if w := lipgloss.Width(stripANSI(got)); w != width {
					t.Errorf("width %d, want %d", w, width)
				}
			}

			// The ground must survive every reset in the row, or the rest of the
			// row falls back to the terminal's own background and is striped.
			if n, resets := strings.Count(got, base), strings.Count(got, "\x1b[0m"); n < resets+1 {
				t.Errorf("ground not restored: %d emits for %d resets", n, resets)
			}

			// A broken sequence is passed through intact, so a consumer that
			// understands it sees exactly what came in. Padding follows it: the
			// sequence is already unterminated and the row is already broken, so
			// there is no rendering to protect, and the frame stays rectangular.
			if c.malformed {
				if !strings.Contains(stripANSI(got), c.line) {
					t.Errorf("malformed sequence not passed through: %q", stripANSI(got))
				}
				if !strings.HasSuffix(got, " ") {
					t.Errorf("row was not filled to width: %q", got)
				}
			}
		})
	}
}

func TestEscapeEndFollowsCSIStructure(t *testing.T) {
	// The parameters of a CSI are 0x30-0x3F and its single final byte is
	// 0x40-0x7E. A scan that stops at the first byte at or above 0x40 cuts a
	// reset in half, because '0' is 0x30 and is not a final byte but a naive
	// test can be written to treat it as one.
	cases := []struct {
		s    string
		want int
	}{
		{"\x1b[0m", 4},
		{"\x1b[m", 3},
		{"\x1b[00m", 5},
		{"\x1b[38;2;1;2;3m", 13},
		{"\x1b[48;5;232m", 11},
		{"\x1b[1;38;2;34;229;139m", 20},
		{"\x1b[200~", 6},
		{"\x1b]0;t\x07", 6},
		{"\x1b]0;t\x1b\\", 7},
		{"\x1b[38;2;1;2;3", -1}, // unterminated
		{"\x1b[", -1},
	}
	for _, c := range cases {
		if got := escapeEnd(c.s, 0); got != c.want {
			t.Errorf("escapeEnd(%q) = %d, want %d", c.s, got, c.want)
		}
	}
}

func TestGroundIsIdentityWithoutColor(t *testing.T) {
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	lipgloss.SetColorProfile(termenv.Ascii)
	th := newTheme(false, nil)

	line := "a row of output"
	if got := ground(th.Base, line, 20); got != line {
		t.Errorf("ground painted with no colour: got %q, want %q unchanged", got, line)
	}
}

func TestEveryFrameRowHasAGround(t *testing.T) {
	// The reason the ground is painted from the finished frame rather than at
	// each call site: on an idle screen most of the rows are content-free, and
	// those are the ones that would otherwise show the terminal's background.
	for _, c := range []struct {
		name    string
		profile termenv.Profile
	}{{"truecolor", termenv.TrueColor}, {"256", termenv.ANSI256}} {
		profile := c.profile
		t.Run(c.name, func(t *testing.T) {
			prev := lipgloss.ColorProfile()
			t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
			lipgloss.SetColorProfile(profile)

			m := modelFor(t, 140, 24, nil)
			m.theme = newTheme(true, nil)
			switch profile {
			case termenv.TrueColor:
				m.theme.Depth = DepthTrueColor
			case termenv.ANSI256:
				m.theme.Depth = Depth256
			}
			m = addBlock(m, &block{command: "p", status: StatusDone, started: nowish()})
			m = refresh(m)

			rows := strings.Split(m.View(), "\n")
			if len(rows) != 24 {
				t.Fatalf("rows %d, want 24", len(rows))
			}
			// Every row is full width, so the ground covers the terminal's edge
			// on all four sides rather than leaving a bare column.
			for i, r := range rows {
				if w := lipgloss.Width(stripANSI(r)); w != 140 {
					t.Errorf("row %d width %d, want 140", i, w)
				}
			}
			for i, r := range rows {
				if n := strings.Count(r, leadingSGR(m.theme.Inset.Render("x"))); n == 0 {
					t.Errorf("row %d has no frame ground: %q", i, r)
				}
			}
			// The transcript is the field the work happens on, one step above
			// the frame, so the panels beside it read as raised off it. The range
			// comes from the height the viewport was actually boxed to: 24 rows
			// less five rows of chrome is nineteen transcript rows and a strip.
			if got, want := m.viewportHeight(m.height), 19; got != want {
				t.Fatalf("transcript height %d, want %d", got, want)
			}
			for i := 1; i <= 19; i++ {
				if !strings.Contains(rows[i], leadingSGR(m.theme.Base.Render("x"))) {
					t.Errorf("transcript row %d is not on the Base field", i)
				}
			}
		})
	}
}

func TestSurfaceLadderIsUsedInOrder(t *testing.T) {
	// A ladder that is defined but not used in order is four arbitrary colours.
	// Each step has to be on screen for its value to mean anything: Inset is the
	// field behind everything, Base the transcript on it, Surface the panels
	// raised off that, and Raised the row the operator is typing into.
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	lipgloss.SetColorProfile(termenv.TrueColor)

	m := modelFor(t, 140, 24, nil)
	m.theme = newTheme(true, nil)
	m = addBlock(m, &block{command: "p", status: StatusDone, started: nowish()})
	m = refresh(m)
	rows := strings.Split(m.View(), "\n")

	sgr := func(s lipgloss.Style) string { return leadingSGR(s.Render("x")) }
	checks := []struct {
		name string
		row  int
		want string
	}{
		{"header is Surface", 0, sgr(m.theme.Surface)},
		{"panel is Surface", 1, sgr(m.theme.Surface)},
		{"transcript is Base", 4, sgr(m.theme.Base)},
		{"input row is Raised", len(rows) - 1, sgr(m.theme.Raised)},
	}
	for _, c := range checks {
		if c.row < 0 || c.row >= len(rows) {
			t.Fatalf("%s: row %d out of range (%d rows)", c.name, c.row, len(rows))
		}
		if !strings.Contains(rows[c.row], c.want) {
			t.Errorf("%s: row %d does not carry its fill\n got %q", c.name, c.row, rows[c.row])
		}
	}

	// The input row is the one surface being touched. It shared Surface with the
	// header and the panels, which left nowhere for the eye to rest while typing.
	if strings.Contains(rows[len(rows)-1], sgr(m.theme.Surface)) {
		t.Errorf("input row still drawn on Surface; the top of the ladder is unused")
	}
}

func TestTextOnEveryGroundIsLegible(t *testing.T) {
	// The palette was chosen dark and the grounds were chosen to be painted, so
	// every ink now sits on four different backgrounds instead of one. Each pair
	// has to clear the bar on its own; none of them does by inheriting the
	// contrast of another.
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	lipgloss.SetColorProfile(termenv.TrueColor)
	p := BaseTheme.resolve().at(DepthTrueColor)

	const (
		textBar  = 4.5
		faintBar = 3.0 // Faint is a dimmed structural ink, not body text
	)
	inks := map[string]struct {
		slot string
		bar  float64
	}{
		"Text": {p.Text, textBar}, "Muted": {p.Muted, textBar}, "Faint": {p.Faint, faintBar},
		"Accent": {p.Accent, textBar}, "AccentLo": {p.AccentLo, textBar}, "AccentHi": {p.AccentHi, textBar},
		"Warning": {p.Warning, textBar}, "Danger": {p.Danger, textBar}, "Cancelled": {p.Cancelled, textBar},
		"Info": {p.Info, textBar}, "Cyan": {p.Cyan, textBar}, "Violet": {p.Violet, textBar}, "High": {p.High, textBar},
	}
	for _, g := range surfaceLadder {
		gslot := slotOfT(p, g)
		gb, ok := slotRGB(gslot)
		if !ok {
			t.Fatalf("%s has bad slot %q", g, gslot)
		}
		for name, ink := range inks {
			ib, ok := slotRGB(ink.slot)
			if !ok {
				t.Fatalf("%s has bad slot %q", name, ink.slot)
			}
			if c := contrast(gb, ib); c < ink.bar {
				t.Errorf("%s on %s is %.2f, want >= %.1f", name, g, c, ink.bar)
			}
		}
	}
}

func TestStructureIsVisibleButNotLoud(t *testing.T) {
	// Rules and borders draw structure. Below the band they are invisible and
	// the panels lose their edges; above it they compete with the text they are
	// meant to organise.
	const lo, hi = 1.2, 2.5
	p := BaseTheme.resolve().at(DepthTrueColor)
	for _, pair := range [][2]string{
		{"Rule", "Inset"}, {"Border", "Inset"},
		{"Rule", "Surface"}, {"Border", "Surface"},
	} {
		fg, ok1 := slotRGB(slotOfT(p, pair[0]))
		bg, ok2 := slotRGB(slotOfT(p, pair[1]))
		if !ok1 || !ok2 {
			t.Fatalf("unreadable slot for %s on %s", pair[0], pair[1])
		}
		if c := contrast(fg, bg); c < lo || c > hi {
			t.Errorf("%s on %s is %.2f, want within [%.1f, %.1f]", pair[0], pair[1], c, lo, hi)
		}
	}
}

func TestNoGroundAboveSixteenColors(t *testing.T) {
	// At sixteen colours the tool does not own its background, so it does not
	// paint one. Approximating the four-step ladder from sixteen swatches would
	// produce four colours that are all nearly the same and a ground that fights
	// the terminal's. Empty means paint nothing, which is what an empty slot
	// resolves to.
	p := BaseTheme.resolve().at(Depth16)
	for _, g := range surfaceLadder {
		if s := slotOfT(p, g); s != "" {
			t.Errorf("%s is %q at 16 colours, want empty", g, s)
		}
	}
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	lipgloss.SetColorProfile(termenv.ANSI)
	th := newTheme(true, nil)
	th.Depth = Depth16
	if got := ground(th.Base, "text", 10); got != "text" {
		t.Errorf("ground painted at 16 colours: got %q", got)
	}
}

// A tool piped to tee must keep producing what it always did. Painting a ground
// is only safe because it cannot reach a pipe: Run refuses to draw into one, so
// View is never called on that path and no fill is ever composed. The refusal is
// the load-bearing part of this, so it is asserted here rather than assumed --
// an escape sequence on a pipe is a corrupted log file, and nothing about the
// pipe would look wrong until someone read it.
func TestGroundNeverReachesAPipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	code, runErr := Run(Config{
		Title:   "QYVORA / PROBE",
		Version: "0.1.0",
		In:      bytes.NewReader(nil),
		Out:     w,
		Runner: &InProcessRunner{
			ToolName: "probe",
			Execute:  func(context.Context, []string) int { return 0 },
			Meta:     testCommands,
		},
	})
	w.Close()
	if runErr == nil {
		t.Fatal("Run drew into a pipe instead of refusing")
	}
	if code == 0 {
		t.Errorf("exit code %d, want non-zero for a non-interactive output", code)
	}
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	if n > 0 {
		t.Errorf("wrote %q to the pipe, want nothing", buf[:n])
	}
}
