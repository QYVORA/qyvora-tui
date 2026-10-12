package domain

import "time"

// Status is the lifecycle state of an execution block.
type Status int

const (
	StatusRunning Status = iota
	StatusDone
	StatusFailed
	StatusCancelled
)

func (s Status) String() string {
	switch s {
	case StatusRunning:
		return "Running"
	case StatusDone:
		return "Done"
	case StatusFailed:
		return "Failed"
	case StatusCancelled:
		return "Cancelled"
	}
	return "Unknown"
}

// Finding is a finding the tool reported, reduced to what the transcript
// shows. The full event payload stays available in the block for expansion.
type Finding struct {
	Title    string
	Severity string
	Target   string
	Detail   string
	At       time.Time
}

// Artifact is a file or resource an execution produced.
type Artifact struct {
	Name string
	Kind string
	Size string
	At   time.Time
}
