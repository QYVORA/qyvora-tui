package domain

import "time"

// Event is the structured event envelope every QYVORA tool emits.
//
// The field set is the shared contract: seven fields, byte-identical across
// all thirteen tools. The TUI decodes this and nothing else. It never reads
// human-readable output, so a tool is free to reword, colourise or drop any
// text it prints without affecting the interface.
type Event struct {
	SchemaVersion string         `json:"schema_version"`
	Timestamp     time.Time      `json:"timestamp"`
	ExecutionID   string         `json:"execution_id"`
	Framework     string         `json:"framework"`
	Level         string         `json:"level"`
	Event         string         `json:"event"`
	Data          map[string]any `json:"data,omitempty"`
}

// Known lifecycle events. These are the ones the interface gives structural
// meaning; anything else still renders, as a generic event row, so a tool may
// add event types without the TUI needing a new release.
const (
	EventExecutionStarted   = "execution.started"
	EventExecutionCompleted = "execution.completed"
	EventExecutionFailed    = "execution.failed"
	EventExecutionCancelled = "execution.cancelled"

	EventFindingDiscovered = "finding.discovered"
	EventProgressUpdated   = "progress.updated"
	EventArtifactCreated   = "artifact.created"
	EventTargetDiscovered  = "target.discovered"
)
