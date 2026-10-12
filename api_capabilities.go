package tui

import "github.com/QYVORA/qyvora-tui/internal/capabilities"

// Capabilities types and helpers are re-exported so the public API is unchanged.
type (
	// Capabilities describes what a tool can do.
	Capabilities = capabilities.Capabilities
	// Capability describes a single capability.
	Capability = capabilities.Capability
	// Parameter describes a capability input parameter.
	Parameter = capabilities.Parameter
	// Entry is a capability entry in the UI.
	Entry = capabilities.Entry
)

// Helpers
var (
	CapabilitiesFrom     = capabilities.CapabilitiesFrom
	NormalizeCapabilities = capabilities.NormalizeCapabilities
	IsLifecycle          = capabilities.IsLifecycle
	IsStage              = capabilities.IsStage
)
