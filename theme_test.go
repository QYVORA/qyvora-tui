package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// The theme tests exist because the failure they guard against is invisible in
// a passing test run and obvious on screen: a terminal with no colour, a
// terminal that cannot render the palette, or a tool that sets one colour and
// gets a blank interface for the other nine.

func TestNewThemeAlwaysProducesUsableStyles(t *testing.T) {
	// Colour on and off, with and without a palette. Every combination has to
	// yield a theme whose styles are usable, because an interface that renders
	// nothing is the worst outcome and the easiest to ship.
	for _, color := range []bool{true, false} {
		for name, p := range map[string]*Palette{"none": nil, "full": &BaseTheme, "partial": &testPalette} {
			th := newTheme(color, p)
			if th.Color != color {
				t.Errorf("color=%t palette=%s: Color = %t, want %t", color, name, th.Color, color)
			}
			// A style with no colour set and no colour allowed is fine; a style
			// that is somehow nil is not. Rendering is the real check.
			if got := th.Value.Render("probe"); stripANSI(got) != "probe" {
				t.Errorf("color=%t palette=%s: Value.Render lost its text: %q", color, name, got)
			}
			if stripANSI(th.Success.Render("ok")) != "ok" {
				t.Errorf("color=%t palette=%s: Success.Render lost its text", color, name)
			}
		}
	}
}

func TestNoColorEnvironmentWins(t *testing.T) {
	// NO_COLOR is a promise to the user's terminal. If a tool's palette or the
	// base theme could override it, the interface would emit escapes a user
	// explicitly asked not to receive, and nothing else in the code would catch
	// that.
	t.Setenv("NO_COLOR", "1")
	if newTheme(true, &BaseTheme).Color {
		t.Fatal("NO_COLOR=1 but the theme is still emitting colour")
	}
	// Unsetting is the only way to get colour back, so this uses Unsetenv
	// rather than an empty value, which is still "set".
	if err := os.Unsetenv("NO_COLOR"); err != nil {
		t.Fatal(err)
	}
	if !newTheme(true, &BaseTheme).Color {
		t.Fatal("colour suppressed with NO_COLOR unset")
	}
	// The variable is presence-based: even NO_COLOR=0 and NO_COLOR= are
	// requests for plain output. Only unsetting it restores colour.
	t.Setenv("NO_COLOR", "0")
	if newTheme(true, nil).Color {
		t.Fatal("NO_COLOR=0 treated as unset; the variable is presence-based")
	}
	t.Setenv("NO_COLOR", "")
	if newTheme(true, nil).Color {
		t.Fatal("NO_COLOR= (empty) treated as unset; the variable is presence-based")
	}
	// The check lives in newTheme, so a caller that forgets it still cannot
	// leak escapes. That is the point of having one place decide.
	th := newTheme(true, &BaseTheme)
	for _, s := range []lipglossStyle{th.Title, th.Value, th.Failed, th.Rule} {
		if strings.Contains(s.Render("x"), "\x1b") {
			t.Fatal("a style emitted an escape sequence under NO_COLOR")
		}
	}
}

func TestPaletteFallbackFillsEverySlot(t *testing.T) {
	// A tool sets one colour. The other nine must still resolve, or the region
	// it did not style disappears.
	th := newTheme(true, &testPalette)
	if th.Palette.Accent != testPalette.Accent {
		t.Errorf("Accent = %q, want the tool's %q", th.Palette.Accent, testPalette.Accent)
	}
	// Every semantic slot must be non-empty; a blank one renders as no style
	// and looks like a bug in the interface rather than an absent palette.
	slots := map[string]string{
		"Accent": th.Palette.Accent, "Warning": th.Palette.Warning,
		"Danger": th.Palette.Danger, "Cancelled": th.Palette.Cancelled,
		"Info": th.Palette.Info, "Text": th.Palette.Text,
		"Muted": th.Palette.Muted, "Faint": th.Palette.Faint,
		"Rule": th.Palette.Rule, "Surface": th.Palette.Surface,
	}
	for slot, v := range slots {
		if v == "" {
			t.Errorf("slot %s is empty with a partial palette", slot)
		}
	}
	// A slot the tool did not set must come from the base theme, not from a
	// default the TUI invented.
	if th.Palette.Danger != BaseTheme.Danger {
		t.Errorf("Danger = %q, want the base theme's %q", th.Palette.Danger, BaseTheme.Danger)
	}
}

func TestNilPaletteYieldsBaseTheme(t *testing.T) {
	th := newTheme(true, nil)
	if th.Palette != BaseTheme {
		t.Errorf("a nil palette did not yield the base theme:\n got %+v\nwant %+v", th.Palette, BaseTheme)
	}
}

func TestLevelStyleCoversTheEnvelopeLevels(t *testing.T) {
	// The envelope's levels are a fixed vocabulary. A level with no style would
	// render in whatever the terminal defaults to, which for an error is the
	// one place where looking ordinary is worst.
	th := newTheme(true, &BaseTheme)
	for _, level := range []string{"info", "warn", "warning", "error", "critical", "debug", "trace", "", "unheard-of"} {
		if stripANSI(th.levelStyle(level).Render("x")) != "x" {
			t.Errorf("level %q lost its text", level)
		}
	}
	if got := stripANSI(th.levelStyle("error").Render("x")); got != "x" {
		t.Errorf("error level rendered %q", got)
	}
	// A level with no meaning must not be styled as an error. Rendering an
	// unknown level in red is a claim about it that nobody made.
	//
	// The styles are compared directly rather than by their rendered output:
	// lipgloss degrades to no colour when its output is not a terminal, so in a
	// test every style renders identically and a comparison on the rendered
	// string would pass whatever the styles actually were.
	if fgOf(th.levelStyle("unheard-of")) == fgOf(th.levelStyle("error")) {
		t.Error("an unknown level is styled as an error")
	}
	if fgOf(th.levelStyle("error")) != fgOf(th.Failed) {
		t.Error("an error is not styled as a failure")
	}
	if fgOf(th.levelStyle("warning")) == fgOf(th.levelStyle("error")) {
		t.Error("a warning is styled as an error")
	}
	// A level the TUI does not know must still be readable, so it falls back to
	// the muted detail style rather than to nothing.
	if fgOf(th.levelStyle("unheard-of")) != fgOf(th.Detail) {
		t.Error("an unknown level did not fall back to the muted style")
	}
}

func TestThemeRendersTextUnchangedInPlainMode(t *testing.T) {
	// The plain theme is the NO_COLOR path, and its output must be the text
	// itself: no escapes at all, so a terminal that honours NO_COLOR and one
	// that merely ignores it behave the same.
	th := newTheme(false, &BaseTheme)
	for name, got := range map[string]string{
		"Value":   th.Value.Render("hello"),
		"Success": th.Success.Render("hello"),
		"Failed":  th.Failed.Render("hello"),
		"Title":   th.Title.Render("hello"),
		"Hint":    th.Hint.Render("hello"),
	} {
		if got != "hello" {
			t.Errorf("%s.Render in plain mode = %q, want the bare text", name, got)
		}
	}
}

// fgOf returns a style's foreground colour, for comparing styles directly.
func fgOf(s lipglossStyle) lipgloss.TerminalColor { return s.GetForeground() }
