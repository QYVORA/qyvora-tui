package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

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

// progressLine is a human-readable progress description from the tool.
type progressLine struct {
	Current int64
	Total   int64
	Percent float64
	Message string
	HasBar  bool
}

// eventRow is one event as it appears inside a block. Raw payloads are kept so
// a block can be expanded to the original event data.
type eventRow struct {
	At     time.Time
	Level  string
	Type   string
	Label  string
	Detail string
	Data   map[string]any
}

// outputCap is how many lines of a command's printed output a block retains.
// A command that prints without limit must not grow the transcript until the
// interface stalls, and the tail is kept because that is where a failure and
// its explanation end up. When the cap trips, the block says so rather than
// silently showing less.
const outputCap = 500

// addOutput records a line of the tool's printed output.
func (b *block) addOutput(line string) {
	b.rawOutput = append(b.rawOutput, line)
	if len(b.output) >= outputCap {
		b.output = b.output[len(b.output)-outputCap+1:]
		b.outputOmitted = true
	}
	b.output = append(b.output, line)
}

// block is one execution: a command the user ran, the events it produced, and
// its result. This is the unit the transcript renders, rather than a flat
// wall of events.
type block struct {
	id       int
	command  string
	args     []string
	started  time.Time
	finished time.Time
	status   Status
	exitCode int

	// expanded reveals the block's raw output in the transcript. A finished
	// run starts collapsed so history stays readable; the executions region
	// expands and collapses it. A running block ignores the flag and always
	// shows live output.
	expanded bool

	rows []eventRow
	// output holds the tool's own printed output, verbatim. It is kept apart
	// from the event rows because it is not an event: it is whatever the
	// command chose to print, shown as written rather than interpreted.
	output []string
	// rawOutput retains every printed line for export; output remains bounded
	// for responsive transcript rendering.
	rawOutput []string
	findings  []Finding
	artifacts []Artifact
	progress  progressLine
	// known is the number of rows rendered without folding, used to keep the
	// collapsed summary honest.
	known int
	err   string
	// cancelled records that the user, rather than the tool, ended this.
	cancelled bool
	// outputOmitted is true when a command printed more than outputCap lines
	// and only the tail is retained; the interface says so rather than hiding
	// the truncation. rowsOmitted is the same for the event log.
	outputOmitted bool
	rowsOmitted   bool
}

func newBlock(id int, args []string) *block {
	return &block{
		id:      id,
		command: commandArgs(args),
		args:    args,
		started: time.Now(),
		status:  StatusRunning,
	}
}

func (b *block) elapsed() time.Duration {
	if b.finished.IsZero() {
		return time.Since(b.started)
	}
	return b.finished.Sub(b.started)
}

// duration renders an elapsed time the way the transcript header shows it.
func duration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%02d:%02d", int(d.Seconds())/60, int(d.Seconds())%60)
	}
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}

// addEvent folds one envelope into the block, extracting the parts the UI
// understands and keeping the payload for the detail view.
//
// Unrecognised event types are stored and rendered generically. A tool is
// allowed to emit event types this build has never heard of, and dropping them
// would make the TUI lose information the JSONL consumer still receives.
// eventCap bounds the event log a block retains. A run that streams events
// for hours must not grow the in-memory history without bound; the tail is
// kept, and the Events view says so. The per-run activity counts are separate,
// so bounding the log does not under-report a run -- it only limits what a
// single block is willing to keep around for expansion.
const eventCap = 2000

func (b *block) addEvent(ev Event) {
	row := eventRow{
		At:     ev.Timestamp,
		Level:  ev.Level,
		Type:   ev.Event,
		Label:  humanEvent(ev.Event),
		Data:   ev.Data,
		Detail: eventDetail(ev),
	}
	if row.At.IsZero() {
		row.At = time.Now()
	}
	if len(b.rows) >= eventCap {
		b.rows = b.rows[len(b.rows)-eventCap+1:]
		b.rowsOmitted = true
	}
	b.rows = append(b.rows, row)
	b.known = len(b.rows)

	switch ev.Event {
	case EventFindingDiscovered, "finding.reported", "vulnerability.found":
		b.addFinding(ev)
	case EventArtifactCreated, "report.generated", "evidence.collected", "file.written":
		b.addArtifact(ev)
	case EventProgressUpdated, "coverage.updated":
		b.addProgress(ev)
	}
}

func (b *block) addFinding(ev Event) {
	f := Finding{
		Title:    firstNonEmpty(dataString(ev.Data, "title"), dataString(ev.Data, "name"), dataString(ev.Data, "id"), "Finding"),
		Severity: strings.ToUpper(firstNonEmpty(dataString(ev.Data, "severity"), dataString(ev.Data, "level"))),
		Target:   firstNonEmpty(dataString(ev.Data, "target"), dataString(ev.Data, "host"), dataString(ev.Data, "url"), dataString(ev.Data, "subject")),
		Detail:   firstNonEmpty(dataString(ev.Data, "description"), dataString(ev.Data, "detail"), dataString(ev.Data, "message")),
		At:       ev.Timestamp,
	}
	b.findings = append(b.findings, f)
}

func (b *block) addArtifact(ev Event) {
	a := Artifact{
		Name: firstNonEmpty(dataString(ev.Data, "path"), dataString(ev.Data, "name"), dataString(ev.Data, "file"), dataString(ev.Data, "artifact")),
		Kind: firstNonEmpty(dataString(ev.Data, "kind"), dataString(ev.Data, "type")),
		Size: firstNonEmpty(dataString(ev.Data, "size"), dataString(ev.Data, "bytes")),
		At:   ev.Timestamp,
	}
	if a.Name == "" {
		// An artifact event with no name is still information; fall back to
		// the event's own description rather than showing a blank row.
		a.Name = firstNonEmpty(dataString(ev.Data, "description"), ev.Event)
	}
	b.artifacts = append(b.artifacts, a)
}

func (b *block) addProgress(ev Event) {
	p := progressLine{Message: dataString(ev.Data, "message")}
	if v, ok := ev.Data["percent"]; ok {
		if f, ok := toFloat(v); ok {
			// Accept both 0-1 and 0-100 conventions; tools disagree on which,
			// and a bar pinned at 1% is worse than no bar at all.
			switch {
			case f > 0 && f <= 1:
				p.Percent = f * 100
			case f > 1 && f <= 100:
				p.Percent = f
			}
			p.HasBar = true
		}
	}
	if v, ok := ev.Data["current"]; ok {
		p.Current, _ = toInt(v)
	}
	if v, ok := ev.Data["total"]; ok {
		p.Total, _ = toInt(v)
	}
	if p.Total > 0 && p.Current > 0 {
		p.Percent = float64(p.Current) / float64(p.Total) * 100
		p.HasBar = true
	}
	b.progress = p
}

// eventDetail picks the most useful single-line summary for an event. It is
// read from known payload keys rather than by formatting the whole map, so a
// tool's own wording surfaces instead of a JSON blob.
func eventDetail(ev Event) string {
	if len(ev.Data) == 0 {
		return ""
	}
	for _, k := range []string{
		"message", "summary", "title", "description", "detail",
		"target", "host", "url", "path", "name", "count", "reason", "error",
	} {
		if v := dataString(ev.Data, k); v != "" {
			return v
		}
	}
	return ""
}

// summarise renders the one-line preview of a completed block.
func (b *block) summarise() string {
	parts := []string{}
	if n := len(b.findings); n > 0 {
		parts = append(parts, plural(n, "finding", "findings"))
	}
	if n := len(b.artifacts); n > 0 {
		parts = append(parts, plural(n, "artifact", "artifacts"))
	}
	if n := b.known; n > 0 {
		parts = append(parts, plural(n, "event", "events"))
	}
	if len(parts) == 0 {
		return "no events"
	}
	return strings.Join(parts, ", ")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func toFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case string:
		var f float64
		if _, err := fmt.Sscanf(t, "%g", &f); err == nil {
			return f, true
		}
	}
	return 0, false
}

func toInt(v any) (int64, bool) {
	if f, ok := toFloat(v); ok {
		return int64(f), true
	}
	return 0, false
}

// sortFindings puts the most severe findings first, so the transcript surfaces
// what matters without the user expanding anything.
func sortFindings(f []Finding) {
	sev := map[string]int{"CRITICAL": 0, "HIGH": 1, "MEDIUM": 2, "MODERATE": 2, "LOW": 3, "INFO": 4, "INFORMATIONAL": 4}
	sort.SliceStable(f, func(i, j int) bool {
		return sev[f[i].Severity] < sev[f[j].Severity]
	})
}
