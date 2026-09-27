package tui

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Theme carries the styles used across the interface.
//
// Colour is decided once, at startup, from NO_COLOR and the detected
// terminal. Every style is a no-op when colour is off, so the same rendering
// code path serves both cases and there is no second implementation to keep in
// step.
type Theme struct {
	Color bool

	Header      lipgloss.Style
	HeaderTitle lipgloss.Style
	Ready       lipgloss.Style
	Running     lipgloss.Style
	Failed      lipgloss.Style
	Cancelled   lipgloss.Style

	Prompt     lipgloss.Style
	Command    lipgloss.Style
	BlockTitle lipgloss.Style
	Label      lipgloss.Style
	Value      lipgloss.Style
	Dim        lipgloss.Style
	Critical   lipgloss.Style
	High       lipgloss.Style
	Medium     lipgloss.Style
	Low        lipgloss.Style
	Info       lipgloss.Style
	Error      lipgloss.Style
	Success    lipgloss.Style
	BarFill    lipgloss.Style
	BarEmpty   lipgloss.Style
	Border     lipgloss.Style
	Hint       lipgloss.Style
}

// newTheme builds the palette, dropping all styling when colour is disabled.
func newTheme(color bool) Theme {
	if !color {
		// With no colour the styles are the identity transform, so every
		// styled string renders as its plain text.
		plain := lipgloss.NewStyle()
		return Theme{Color: false, Header: plain, HeaderTitle: plain, Ready: plain,
			Running: plain, Failed: plain, Cancelled: plain, Prompt: plain,
			Command: plain, BlockTitle: plain, Label: plain, Value: plain,
			Dim: plain, Critical: plain, High: plain, Medium: plain,
			Low: plain, Info: plain, Error: plain, Success: plain,
			BarFill: plain, BarEmpty: plain, Border: plain, Hint: plain}
	}

	return Theme{
		Color:       true,
		Header:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62")),
		HeaderTitle: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")),
		Ready:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42")),
		Running:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214")),
		Failed:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196")),
		Cancelled:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("208")),

		Prompt:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")),
		Command:    lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		BlockTitle: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")),
		Label:      lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		Value:      lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		Dim:        lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		Critical:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("199")),
		High:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("203")),
		Medium:     lipgloss.NewStyle().Foreground(lipgloss.Color("215")),
		Low:        lipgloss.NewStyle().Foreground(lipgloss.Color("78")),
		Info:       lipgloss.NewStyle().Foreground(lipgloss.Color("110")),
		Error:      lipgloss.NewStyle().Foreground(lipgloss.Color("203")),
		Success:    lipgloss.NewStyle().Foreground(lipgloss.Color("42")),
		BarFill:    lipgloss.NewStyle().Foreground(lipgloss.Color("42")),
		BarEmpty:   lipgloss.NewStyle().Foreground(lipgloss.Color("238")),
		Border:     lipgloss.NewStyle().Foreground(lipgloss.Color("238")),
		Hint:       lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
	}
}

// colorEnabled decides whether to emit colour.
//
// NO_COLOR wins over everything, including an explicit TERM that promises
// colour: the convention is that setting it is a request for plain output, and
// a library overriding that would break the caller's tooling.
func colorEnabled(out *os.File) bool {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	if v := os.Getenv("NO_COLOR"); v != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	// A file that is not a terminal cannot render styling meaningfully, and
	// emitting escape codes into a pipe is a defect, not a nicety.
	return isTerminal(out)
}

// severityStyle maps a severity label onto a style, defaulting to info.
func (t Theme) severityStyle(sev string) lipgloss.Style {
	switch strings.ToUpper(sev) {
	case "CRITICAL":
		return t.Critical
	case "HIGH":
		return t.High
	case "MEDIUM", "MODERATE":
		return t.Medium
	case "LOW":
		return t.Low
	default:
		return t.Info
	}
}

// statusStyle maps a status onto the style the header dot uses.
func (t Theme) statusStyle(s Status) lipgloss.Style {
	switch s {
	case StatusRunning:
		return t.Running
	case StatusFailed:
		return t.Failed
	case StatusCancelled:
		return t.Cancelled
	default:
		return t.Ready
	}
}
