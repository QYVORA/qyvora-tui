package tui

import (
	"math"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// ColorDepth is how much colour the terminal can actually render.
//
// It exists because lipgloss's own degradation of a hex value to the nearest
// available colour is not good enough for this interface. Measured against the
// QYVORA ground, its nearest-colour answer turns a quiet divider into a bright
// green line and turns the CRITICAL badge's red background into black. The
// interface draws layered surfaces and severity badges for a living; guessing
// which of the 256 colours is nearest is the one thing it must not do. So the
// depth is resolved once at startup and every slot is then replaced by an
// explicit index from a table someone checked.
type ColorDepth int

const (
	// DepthNone means no colour at all: NO_COLOR, TERM=dumb, or a pipe.
	DepthNone ColorDepth = iota
	// Depth16 is the sixteen-colour palette.
	Depth16
	// Depth256 is the 256-colour palette.
	Depth256
	// DepthTrueColor is 24-bit colour.
	DepthTrueColor
)

func (d ColorDepth) String() string {
	switch d {
	case Depth16:
		return "16"
	case Depth256:
		return "256"
	case DepthTrueColor:
		return "truecolor"
	default:
		return "none"
	}
}

// supportsFills reports whether the depth can carry a background colour the
// interface controls. At sixteen colours it cannot: the background is whatever
// the user's terminal is using, and painting a field on top of it is how a
// label ends up unreadable. Layering at that depth is carried by borders and
// rules instead.
func (d ColorDepth) supportsFills() bool { return d >= Depth256 }

// detectColorDepth reports the depth lipgloss resolved from the environment.
//
// This is deliberately lipgloss's answer rather than a second probe of the
// environment: the styles in this package are rendered by lipgloss, so the only
// depth that matters is the one lipgloss will honour. Two answers could disagree
// and the interface would be right about one of them.
func detectColorDepth() ColorDepth {
	// Note the constants' numeric values: termenv orders them TrueColor first.
	// A switch rather than a comparison keeps that detail out of this file.
	switch lipgloss.ColorProfile() {
	case termenv.TrueColor:
		return DepthTrueColor
	case termenv.ANSI256:
		return Depth256
	case termenv.ANSI:
		return Depth16
	default:
		return DepthNone
	}
}

// RGB is a red-green-blue triple. Terminal palette indices are resolved to one
// of these so a colour can be measured rather than assumed.
type RGB struct{ R, G, B uint8 }

// Color256 is an index into a 256-colour terminal palette.
type Color256 uint8

// Color16 is an index into the sixteen base terminal colours.
type Color16 uint8

// RGB returns the colour a 256-colour terminal shows for this index.
func (c Color256) RGB() RGB { return rgbAt256(uint8(c)) }

// RGB returns the colour a sixteen-colour terminal shows for this index.
func (c Color16) RGB() RGB { return rgbAt16(uint8(c)) }

// ansi16 is the sixteen base colours as xterm and every terminal that inherited
// its palette define them. The two halves differ: 0-7 are dim and 8-15 are
// bright, which is why the interface's sixteen-colour map uses 37 for muted text
// and 97 for normal text rather than treating the ramp as one scale.
var ansi16 = [16]RGB{
	{R: 0, G: 0, B: 0},       // 0 black
	{R: 205, G: 0, B: 0},     // 1 red
	{R: 0, G: 205, B: 0},     // 2 green
	{R: 205, G: 205, B: 0},   // 3 yellow
	{R: 0, G: 0, B: 238},     // 4 blue
	{R: 205, G: 0, B: 205},   // 5 magenta
	{R: 0, G: 205, B: 205},   // 6 cyan
	{R: 229, G: 229, B: 229}, // 7 white
	{R: 127, G: 127, B: 127}, // 8 bright black
	{R: 255, G: 0, B: 0},     // 9 bright red
	{R: 0, G: 255, B: 0},     // 10 bright green
	{R: 255, G: 255, B: 0},   // 11 bright yellow
	{R: 92, G: 92, B: 255},   // 12 bright blue
	{R: 255, G: 0, B: 255},   // 13 bright magenta
	{R: 0, G: 255, B: 255},   // 14 bright cyan
	{R: 255, G: 255, B: 255}, // 15 bright white
}

// cubeLevels and grayLevels reconstruct the 6x6x6 colour cube and the
// greyscale ramp, so a table entry can be verified by reading its RGB back
// rather than by trusting a comment.
var (
	cubeLevels = [6]uint8{0, 95, 135, 175, 215, 255}
	grayLevels = [24]uint8{8, 18, 28, 38, 48, 58, 68, 78, 88, 98, 108, 118, 128, 138, 148, 158, 168, 178, 188, 198, 208, 218, 228, 238}
)

func rgbAt256(idx uint8) RGB {
	switch {
	case idx < 16:
		return ansi16[idx]
	case idx < 232:
		n := int(idx) - 16
		v := cubeLevels
		return RGB{R: v[n/36], G: v[(n/6)%6], B: v[n%6]}
	case int(idx) < len(grayLevels)+232:
		v := grayLevels[idx-232]
		return RGB{R: v, G: v, B: v}
	default:
		return RGB{}
	}
}

func rgbAt16(idx uint8) RGB { return ansi16[idx&0x0f] }

// luminance is the WCAG relative luminance of a colour. It is what every
// contrast assertion in this package is stated in, because "is this readable on
// that ground" is a luminance question and not a hue question.
func (c RGB) luminance() float64 {
	return 0.2126*linear(c.R) + 0.7152*linear(c.G) + 0.0722*linear(c.B)
}

func linear(v uint8) float64 {
	s := float64(v) / 255
	if s <= 0.03928 {
		return s / 12.92
	}
	return math.Pow((s+0.055)/1.055, 2.4)
}

// contrast is the WCAG contrast ratio between two colours, from 1 (identical) to
// 21 (black on white).
func contrast(a, b RGB) float64 {
	la, lb := a.luminance(), b.luminance()
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}
