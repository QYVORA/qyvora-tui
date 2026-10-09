package tui

import (
	"strings"
	"testing"
)

func TestThinBannerCarriesNameAndTagline(t *testing.T) {
	got := ThinBanner("anansi", "Attack Surface Intelligence Engine")
	if !strings.Contains(got, "ANANSI") {
		t.Errorf("ThinBanner = %q, want the name", got)
	}
	if !strings.Contains(got, "Attack Surface Intelligence Engine") {
		t.Errorf("ThinBanner = %q, want the tagline", got)
	}
	if strings.Count(got, "\n") != 0 {
		t.Errorf("ThinBanner = %q, want a single line", got)
	}
}

func TestThinBannerCapitalisesName(t *testing.T) {
	if got := ThinBanner("shaka", ""); got != "── SHAKA ──" {
		t.Errorf("ThinBanner = %q, want %q", got, "── SHAKA ──")
	}
}

func TestRenderCLIArtReturnsArtWhenItFits(t *testing.T) {
	art := []string{"  hello", "  world"}
	got := RenderCLIArt(art, 80, "x", "")
	if len(got) != 2 || got[0] != "hello" || got[1] != "world" {
		t.Errorf("RenderCLIArt = %#v, want trimmed art", got)
	}
}

func TestRenderCLIArtFallsBackToThinBannerWhenNarrow(t *testing.T) {
	art := []string{"hello world"}
	got := RenderCLIArt(art, 5, "anansi", "tagline")
	if len(got) != 1 || got[0] != "ANANSI" {
		t.Errorf("RenderCLIArt = %#v, want the bare name on a tiny terminal", got)
	}
}

func TestRenderCLIArtUnmeasuredWidthKeepsArt(t *testing.T) {
	art := []string{"  hello"}
	got := RenderCLIArt(art, 0, "anansi", "tagline")
	if len(got) != 1 || got[0] != "hello" {
		t.Errorf("RenderCLIArt with width 0 = %#v, want untrimmed-is-trimmed art", got)
	}
}

func TestRenderCLIArtStripsCommonLeadingColumn(t *testing.T) {
	art := []string{"  a", "    b"}
	got := RenderCLIArt(art, 80, "x", "")
	if got[0] != "a" || got[1] != "  b" {
		t.Errorf("Art with per-row indents = %#v, want the common indent removed", got)
	}
}

func TestRenderCLIArtFitThinDegradesGracefully(t *testing.T) {
	art := []string{"a monarch long enough that a thin line must wrap"}
	got := RenderCLIArt(art, 8, "anansi", "a long descriptor that also gets dropped")
	if len(got) != 1 || got[0] != "ANANSI" {
		t.Errorf("narrow output = %#v, want the bare name to fit", got)
	}
}

func TestRenderCLIArtEmptyArtIsHandled(t *testing.T) {
	if got := RenderCLIArt(nil, 0, "anansi", ""); len(got) != 0 {
		t.Errorf("RenderCLIArt(nil) = %#v, want empty", got)
	}
}
