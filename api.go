package tui

import "github.com/QYVORA/qyvora-tui/internal/domain"

// The shared domain types live in internal/domain so the subpackages (commands,
// capabilities, form, render) can depend on them without importing the root
// package. They are re-exported here so the module's public API is unchanged.
type (
	// Command describes a single command for completion and help.
	Command = domain.Command
	// Flag describes a command-line flag.
	Flag = domain.Flag
	// Example shows a usage example with optional explanatory note.
	Example = domain.Example
	// Status is the lifecycle state of an execution block.
	Status = domain.Status
	// Finding is a finding the tool reported, reduced to what the transcript shows.
	Finding = domain.Finding
	// Artifact is a file or resource an execution produced.
	Artifact = domain.Artifact
	// Event is the structured event envelope every QYVORA tool emits.
	Event = domain.Event
)

// Execution lifecycle states, re-exported from the domain package.
const (
	StatusRunning   = domain.StatusRunning
	StatusDone      = domain.StatusDone
	StatusFailed    = domain.StatusFailed
	StatusCancelled = domain.StatusCancelled
)

// Known lifecycle and payload event types, re-exported from the domain package.
const (
	EventExecutionStarted   = domain.EventExecutionStarted
	EventExecutionCompleted = domain.EventExecutionCompleted
	EventExecutionFailed    = domain.EventExecutionFailed
	EventExecutionCancelled = domain.EventExecutionCancelled

	EventFindingDiscovered = domain.EventFindingDiscovered
	EventProgressUpdated   = domain.EventProgressUpdated
	EventArtifactCreated   = domain.EventArtifactCreated
	EventTargetDiscovered  = domain.EventTargetDiscovered
)
