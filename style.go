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
func fg(hex string) lipglossStyle {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(hex))
}

// bg returns a background-only style for filled terminal surfaces.
func bg(hex string) lipglossStyle {
	return lipgloss.NewStyle().Background(lipgloss.Color(hex))
}

func badgeStyle(foreground, background string) lipglossStyle {
	return lipgloss.NewStyle().Bold(true).
		Foreground(lipgloss.Color(foreground)).
		Background(lipgloss.Color(background)).
		Padding(0, 1)
}

// bold returns a bold foreground style.
func bold(hex string) lipglossStyle {
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(hex))
}

// newPlainStyleImpl is the concrete identity transform. It is named here so
// newPlainStyle in the theme reads as intent rather than as a wrapper.
func newPlainStyleImpl() lipglossStyle { return lipgloss.NewStyle() }

// lookupEnv and getenv exist so the environment can be inspected through one
// place, which keeps NO_COLOR handling testable without mutating the process.
func lookupEnv(key string) (string, bool) { return os.LookupEnv(key) }

func getenv(key string) string { return os.Getenv(key) }
