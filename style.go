package tui

import (
	"os"

	"github.com/charmbracelet/lipgloss"
)

// The shared layer refers to lipgloss through these aliases so a rendering
// component's dependency on a styling library is visible and named, rather than
// a dozen files each importing the concrete type.
type (
	lipglossStyle = lipgloss.Style
	osFile        = os.File
)

// fg returns a foreground-only style.
func fg(slot string) lipglossStyle {
	return lipgloss.NewStyle().Foreground(paletteColor(slot))
}

// bg returns a background-only style for filled terminal surfaces.
func bg(slot string) lipglossStyle {
	return lipgloss.NewStyle().Background(paletteColor(slot))
}

func badgeStyle(foreground, background string) lipglossStyle {
	return lipgloss.NewStyle().Bold(true).
		Foreground(paletteColor(foreground)).
		Background(paletteColor(background)).
		Padding(0, 1)
}

// bold returns a bold foreground style.
func bold(slot string) lipglossStyle {
	return lipgloss.NewStyle().Bold(true).Foreground(paletteColor(slot))
}

// fill returns a background-only style for a painted surface, or the identity
// transform when the palette slot is empty.
//
// An empty slot is not a mistake: at sixteen colours the palette deliberately
// has no ground, because painting a field the user cannot choose makes every
// label on it a guess. Returning the identity transform means "leave the
// terminal's own background alone", which is the correct rendering rather than a
// degraded one.
func fill(slot string) lipglossStyle {
	if slot == "" {
		return newPlainStyle()
	}
	return bg(slot)
}

// pillStyle builds the filled label used for status, counts and severities.
//
// Above sixteen colours a pill is ink on a field. At sixteen colours there is no
// field to sit on, so a pill drops to bold ink on the terminal's own background:
// a near-black field would make the ink muddy and the cap glyphs unreadable, and
// a label that cannot be read is worse than an unlabelled one.
//
// A pill whose field slot is empty lands here too, whatever the depth. That is
// the 256-colour case for every field except CRITICAL: the cube has no dark green
// dark enough to sit under #22E58B and stay a field rather than a hole, so the
// pill carries its information in weight instead.
func pillStyle(d ColorDepth, ink, field string) lipglossStyle {
	if field == "" || !d.supportsFills() {
		return boldInk(ink)
	}
	return lipgloss.NewStyle().Bold(true).
		Foreground(paletteColor(ink)).
		Background(paletteColor(field))
}

// criticalPillStyle is the one pill that keeps its field at every depth.
//
// CRITICAL has to be the loudest thing on screen, and weight alone is not always
// loud enough when it is competing with four other coloured words. The palette
// leaves CriticalBg populated even at sixteen colours, where every other fill is
// gone, so that this badge is the only filled label a reduced-depth terminal
// will draw.
func criticalPillStyle(ink, field string) lipglossStyle {
	if field == "" {
		return boldInk(ink)
	}
	return lipgloss.NewStyle().Bold(true).
		Foreground(paletteColor(ink)).
		Background(paletteColor(field))
}

func boldInk(ink string) lipglossStyle {
	return lipgloss.NewStyle().Bold(true).Foreground(paletteColor(ink))
}

// newPlainStyleImpl is the concrete identity transform. It is named here so
// newPlainStyle in the theme reads as intent rather than as a wrapper.
func newPlainStyleImpl() lipglossStyle { return lipgloss.NewStyle() }

// lookupEnv and getenv exist so the environment can be inspected through one
// place, which keeps NO_COLOR handling testable without mutating the process.
func lookupEnv(key string) (string, bool) { return os.LookupEnv(key) }

func getenv(key string) string { return os.Getenv(key) }
