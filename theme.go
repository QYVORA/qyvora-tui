package tui

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The QYVORA palette.
//
// The accent is the QYVORA green. Everything else is a near-black ground with
// muted greys, so colour is reserved for the two things worth noticing: the
// prompt, and the state of the work. A terminal agent spends most of its life
// showing text, and colouring all of it is how a session becomes unreadable.
const (
	accentHex    = "#06B66F" // QYVORA green: prompt, rules, success
	warnHex      = "#D9A441"
	dangerHex    = "#E5484D"
	cancelledHex = "#8B8FA3"
	infoHex      = "#6E7B8B"

	textHex  = "#E6E8EB"
	mutedHex = "#7A8290"
	faintHex = "#4A5058"
	ruleHex  = "#262B33"
)

// Theme carries the styles used across the interface.
//
// Colour is decided once, at startup, from NO_COLOR and the detected terminal.
// Every style is a no-op when colour is off, so there is one rendering path
// rather than a plain-text variant that could drift from it.
type Theme struct {
	Color bool

	// Identity.
	Title   lipgloss.Style
	Version lipgloss.Style
	Rule    lipgloss.Style

	// Status. These are the only saturated colours in the resting interface.
	Ready     lipgloss.Style
	Running   lipgloss.Style
	Failed    lipgloss.Style
	Cancelled lipgloss.Style

	// Session.
	Command lipgloss.Style
	Arg     lipgloss.Style
	Group   lipgloss.Style
	Label   lipgloss.Style
	Value   lipgloss.Style
	Detail  lipgloss.Style
	Success lipgloss.Style

	// Composer.
	Prompt lipgloss.Style
	Hint   lipgloss.Style
	Badge  lipgloss.Style

	// Severity.
	Critical lipgloss.Style
	High     lipgloss.Style
	Medium   lipgloss.Style
	Low      lipgloss.Style
	Info     lipgloss.Style

	// Progress.
	BarFill  lipgloss.Style
	BarEmpty lipgloss.Style
}

// newTheme builds the palette, dropping all styling when colour is disabled.
func newTheme(color bool) Theme {
	if !color {
		// With no colour each style is the identity transform, so every styled
		// string renders as its plain text and the interface stays legible
		// without a second implementation.
		plain := lipgloss.NewStyle()
		return Theme{
			Color: false, Title: plain, Version: plain, Rule: plain,
			Ready: plain, Running: plain, Failed: plain, Cancelled: plain,
			Command: plain, Arg: plain, Group: plain, Label: plain,
			Value: plain, Detail: plain, Success: plain, Prompt: plain,
			Hint: plain, Badge: plain, Critical: plain, High: plain,
			Medium: plain, Low: plain, Info: plain, BarFill: plain,
			BarEmpty: plain,
		}
	}

	return Theme{
		Color:   true,
		Title:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(accentHex)),
		Version: lipgloss.NewStyle().Foreground(lipgloss.Color(faintHex)),
		Rule:    lipgloss.NewStyle().Foreground(lipgloss.Color(ruleHex)),

		Ready:     lipgloss.NewStyle().Foreground(lipgloss.Color(accentHex)),
		Running:   lipgloss.NewStyle().Foreground(lipgloss.Color(accentHex)),
		Failed:    lipgloss.NewStyle().Foreground(lipgloss.Color(dangerHex)),
		Cancelled: lipgloss.NewStyle().Foreground(lipgloss.Color(cancelledHex)),

		Command: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(textHex)),
		Arg:     lipgloss.NewStyle().Foreground(lipgloss.Color(accentHex)),
		Group:   lipgloss.NewStyle().Foreground(lipgloss.Color(mutedHex)),
		Label:   lipgloss.NewStyle().Foreground(lipgloss.Color(mutedHex)),
		Value:   lipgloss.NewStyle().Foreground(lipgloss.Color(textHex)),
		Detail:  lipgloss.NewStyle().Foreground(lipgloss.Color(faintHex)),
		Success: lipgloss.NewStyle().Foreground(lipgloss.Color(accentHex)),

		Prompt: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(accentHex)),
		Hint:   lipgloss.NewStyle().Foreground(lipgloss.Color(faintHex)),
		Badge:  lipgloss.NewStyle().Foreground(lipgloss.Color(infoHex)),

		Critical: lipgloss.NewStyle().Foreground(lipgloss.Color(dangerHex)),
		High:     lipgloss.NewStyle().Foreground(lipgloss.Color(warnHex)),
		Medium:   lipgloss.NewStyle().Foreground(lipgloss.Color(warnHex)),
		Low:      lipgloss.NewStyle().Foreground(lipgloss.Color(infoHex)),
		Info:     lipgloss.NewStyle().Foreground(lipgloss.Color(mutedHex)),

		BarFill:  lipgloss.NewStyle().Foreground(lipgloss.Color(accentHex)),
		BarEmpty: lipgloss.NewStyle().Foreground(lipgloss.Color(ruleHex)),
	}
}

// colorEnabled decides whether to emit colour.
//
// NO_COLOR wins over everything, including a TERM that promises colour: setting
// it is a request for plain output, and a library overriding that would break
// whatever the caller is piping into.
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
	// A stream that is not a terminal cannot render styling meaningfully, and
	// emitting escape codes into a pipe is a defect rather than a nicety.
	return isTerminal(out)
}

// severityStyle maps a severity label onto a style, defaulting to muted.
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

// severityTag renders a severity as a fixed-width tag, so findings line up in a
// column no matter how long the label is.
func (t Theme) severityTag(sev string) string {
	switch strings.ToUpper(sev) {
	case "CRITICAL":
		return "CRIT"
	case "HIGH":
		return "HIGH"
	case "MEDIUM", "MODERATE":
		return "MED "
	case "LOW":
		return "LOW "
	default:
		return "INFO"
	}
}

// statusStyle maps an execution status onto its style.
func (t Theme) statusStyle(s Status) lipgloss.Style {
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
