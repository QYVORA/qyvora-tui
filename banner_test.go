package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// qyvoraTools is every tool that ships a wordmark. It is the list the banner
// ladder is tested against, so a new framework name cannot be added without
// being drawn.
var qyvoraTools = []string{
	"AKSUM", "AMANIRENAS", "AMINA", "ANANSI", "IMHOTEP", "JABARI", "KUSH",
	"MANSA", "NZINGA", "SEKHMET", "SHAKA", "SUNDIATA", "TIMBUKTU", "TOHA3EE",
}

// bannerToolsWithPrefixes is how the frameworks actually spell the names.
var bannerToolsWithPrefixes = []string{
	"qyvora-aksum", "qyvora-amanirenas", "qyvora-amina", "qyvora-anansi",
	"qyvora-imhotep", "qyvora-jabari", "qyvora-kush", "qyvora-mansa",
	"qyvora-nzinga", "qyvora-sekhmet", "qyvora-shaka", "qyvora-sundiata",
	"qyvora-timbuktu", "qyvora-toha3ee",
}

func TestBannerEveryToolNameIsDrawableAtFullWidth(t *testing.T) {
	for _, name := range qyvoraTools {
		got := RenderBanner(Banner{Tool: name}, 60, 0)
		if got.IsDropped() {
			t.Errorf("%s: dropped at 60 columns; every tool must draw", name)
			continue
		}
		if got.Variant != VariantFull {
			t.Errorf("%s: variant = %s, want full at 60 columns", name, got.Variant)
		}
		if got.Width > maxFullWidth {
			t.Errorf("%s: width = %d, over the %d column budget", name, got.Width, maxFullWidth)
		}
		if got.Height != fullHeight {
			t.Errorf("%s: height = %d, want %d", name, got.Height, fullHeight)
		}
	}
}

func TestBannerPrefixedNamesDrawTheSameArt(t *testing.T) {
	for i, prefixed := range bannerToolsWithPrefixes {
		want := RenderBanner(Banner{Tool: qyvoraTools[i]}, 60, 0)
		got := RenderBanner(Banner{Tool: prefixed}, 60, 0)
		if got.Width != want.Width || got.Height != want.Height {
			t.Errorf("%s: %dx%d, want %dx%d from %s", prefixed, got.Width, got.Height, want.Width, want.Height, qyvoraTools[i])
			continue
		}
		for r := range want.ASCII {
			if got.ASCII[r] != want.ASCII[r] {
				t.Errorf("%s: row %d = %q, want %q", prefixed, r, got.ASCII[r], want.ASCII[r])
			}
		}
	}
}

func TestBannerNormalisesHowFrameworksSpellTheirNames(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"qyvora-amanirenas", "AMANIRENAS"},
		{"qyvora/toha3ee", "TOHA3EE"},
		{"qyvora_toha3ee", "TOHA3EE"},
		{"QYVORA-SEKHMET", "SEKHMET"},
		{"qyvora-amanirenas ", "AMANIRENAS"},
		{"bin/qyvora-nzinga", "NZINGA"},
		// A hyphen that is not a QYVORA prefix must not eat a syllable: only the
		// leading product segment is stripped, the rest of the name survives.
		{"sun-diata", "SUNDIATA"},
		{"sun diata", "SUNDIATA"},
		{"toha3ee", "TOHA3EE"},
		// A bare "qyvora" has no product segment to strip, so it draws as
		// itself rather than as nothing.
		{"qyvora", "QYVORA"},
		{"qyvora-", ""},
	} {
		if got := normaliseName(tc.in); got != tc.want {
			t.Errorf("normaliseName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestBannerPicksTheRichestVariantThatFits(t *testing.T) {
	for _, tc := range []struct {
		tool  string
		width int
		want  Variant
	}{
		{"AMANIRENAS", 60, VariantFull},    // 59 wide
		{"AMANIRENAS", 58, VariantStacked}, // 29 wide
		{"AMANIRENAS", 28, VariantMarquee}, // 14 wide
		{"AMANIRENAS", 13, VariantNone},
		{"KUSH", 60, VariantFull},    // 23 wide
		{"KUSH", 22, VariantMarquee}, // 8 wide
		{"KUSH", 7, VariantNone},
	} {
		got := RenderBanner(Banner{Tool: tc.tool}, tc.width, 0)
		if got.Variant != tc.want {
			t.Errorf("%s at %d columns: variant = %s, want %s", tc.tool, tc.width, got.Variant, tc.want)
		}
	}
}

func TestBannerHonoursAHeightConstraint(t *testing.T) {
	// The full face needs five rows. Asked for three, the ladder must drop to
	// the compact face rather than draw something that will not fit.
	got := RenderBanner(Banner{Tool: "SEKHMET"}, 60, 3)
	if got.Variant != VariantCompact {
		t.Fatalf("variant = %s, want compact when only three rows are offered", got.Variant)
	}
	if got.Height != compactHeight {
		t.Fatalf("height = %d, want %d", got.Height, compactHeight)
	}
	if want := RenderBanner(Banner{Tool: "SEKHMET"}, 60, 0).Width; got.Width != want {
		t.Fatalf("compact width = %d, want the full face's %d; the compact face trades rows, not columns", got.Width, want)
	}
}

func TestBannerNeverReturnsArtWiderThanItWasGiven(t *testing.T) {
	for _, name := range qyvoraTools {
		for width := 8; width <= 64; width++ {
			got := RenderBanner(Banner{Tool: name}, width, 0)
			if got.IsDropped() {
				continue
			}
			if got.Width > width {
				t.Errorf("%s at %d columns: art is %d wide", name, width, got.Width)
			}
			for i, line := range got.ASCII {
				if n := lipgloss.Width(line); n > width {
					t.Errorf("%s at %d columns: row %d is %d wide", name, width, i, n)
				}
			}
		}
	}
}

func TestBannerNeverExceedsTwelveRows(t *testing.T) {
	for _, name := range qyvoraTools {
		for width := 8; width <= 64; width++ {
			if got := RenderBanner(Banner{Tool: name}, width, 0); got.Height > maxBannerRows {
				t.Errorf("%s at %d columns: %d rows", name, width, got.Height)
			}
		}
	}
}

func TestBannerDropsRatherThanTruncating(t *testing.T) {
	// Too narrow for even a marquee. The one thing the ladder must never do is
	// return a partial wordmark, because the operator would have to read past
	// the damage to find the tool name.
	got := RenderBanner(Banner{Tool: "AMANIRENAS"}, 6, 0)
	if !got.IsDropped() {
		t.Fatalf("returned %q at 6 columns; want nothing", got.ASCII)
	}
	if got.Variant != VariantNone {
		t.Errorf("variant = %s, want none", got.Variant)
	}
}

func TestBannerMarqueeCarriesTheName(t *testing.T) {
	got := RenderBanner(Banner{Tool: "NZINGA"}, 20, 0)
	if !strings.Contains(got.ASCII[0], "NZINGA") {
		t.Fatalf("marquee %q does not carry the name", got.ASCII[0])
	}
	if got.ASCII[0] != glyphRightHalf+" NZINGA "+glyphLeftHalf {
		t.Errorf("marquee = %q, want capped sides", got.ASCII[0])
	}
}

func TestBannerExplicitArtIsMeasuredNotGenerated(t *testing.T) {
	art := []string{"###", "#.#", "###"}
	got := RenderBanner(Banner{Art: art}, 10, 0)
	if got.Width != 3 || got.Height != 3 {
		t.Fatalf("measured %dx%d, want 3x3", got.Width, got.Height)
	}
	for i, l := range got.ASCII {
		if l != art[i] {
			t.Errorf("row %d = %q, want %q", i, l, art[i])
		}
	}
	// Explicit art that does not fit is refused, not squeezed.
	if got := RenderBanner(Banner{Art: art}, 2, 0); !got.IsDropped() {
		t.Fatalf("oversized explicit art was accepted: %q", got.ASCII)
	}
	// Explicit art is trimmed to an offered height, because a caller that asked
	// for three rows wants three rows.
	if got := RenderBanner(Banner{Art: art}, 10, 2); got.Height != 2 {
		t.Fatalf("height = %d, want 2", got.Height)
	}
}

func TestBannerTaglineIsDroppedRatherThanClipped(t *testing.T) {
	long := strings.Repeat("x", 80)
	got := RenderBanner(Banner{Tool: "KUSH", Tagline: long}, 60, 0)
	if strings.Contains(strings.Join(got.ASCII, "\n"), long[:20]) {
		t.Fatal("a tagline wider than the art was drawn")
	}
	for _, l := range got.ASCII {
		if strings.HasPrefix(l, long[:8]) {
			t.Fatalf("clipped tagline row: %q", l)
		}
	}
	short := RenderBanner(Banner{Tool: "KUSH", Tagline: "Qyvora Kush"}, 60, 0)
	if !strings.Contains(strings.Join(short.ASCII, "\n"), "Qyvora Kush") {
		t.Fatalf("tagline missing: %q", short.ASCII)
	}
	if short.Height != fullHeight+1 {
		t.Fatalf("height = %d, want %d", short.Height, fullHeight+1)
	}
}

func TestBannerIsStableAcrossCalls(t *testing.T) {
	// Map iteration order must not reach the art. A wordmark that reshuffles
	// between frames is the most visible possible rendering fault.
	first := RenderBanner(Banner{Tool: "SUNDIATA"}, 60, 0)
	for i := 0; i < 50; i++ {
		got := RenderBanner(Banner{Tool: "SUNDIATA"}, 60, 0)
		for r := range first.ASCII {
			if got.ASCII[r] != first.ASCII[r] {
				t.Fatalf("row %d changed on call %d: %q then %q", r, i, first.ASCII[r], got.ASCII[r])
			}
		}
	}
}

func TestBannerBlankToolNameDrops(t *testing.T) {
	for _, in := range []string{"", "   ", "-", "/", "???"} {
		if got := RenderBanner(Banner{Tool: in}, 60, 0); !got.IsDropped() {
			t.Errorf("%q drew %q", in, got.ASCII)
		}
	}
}

func TestBannerWordmarkRowsAreEqualWidth(t *testing.T) {
	// Every glyph is a fixed five columns, so a row must be exactly 6n-1 wide.
	// A ragged row means a glyph table entry drifted and the art has a notch in it.
	for _, name := range qyvoraTools {
		got := RenderBanner(Banner{Tool: name}, 60, 0)
		want := len(name)*6 - 1
		if got.Width != want {
			t.Errorf("%s: width = %d, want %d", name, got.Width, want)
		}
		for i, l := range got.ASCII {
			if n := lipgloss.Width(l); n != want {
				t.Errorf("%s: row %d is %d wide, want %d", name, i, n, want)
			}
		}
	}
}
