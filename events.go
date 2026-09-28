package tui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

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

// readEvents decodes JSONL from r, calling fn for each event and text for each
// line of plain output the tool wrote alongside them.
//
// The second callback matters. A tool is not only a source of events: it also
// prints things that *are* the answer -- a version block, a capability table, a
// dry-run listing. Because the TUI points the tool's standard output at the
// stream it reads, that text arrives here, and discarding it would leave a
// command that ran correctly looking like one that produced nothing.
//
// A line that is not JSON is passed to text; a line that is JSON but is not a
// well-formed envelope is counted as malformed and skipped. The distinction is
// deliberate: one is a tool's output, the other is a bug or a truncated write.
//
// It returns when r reaches EOF or errors. Decoding is deliberately lenient:
// a line that is not a well-formed envelope is counted and skipped rather
// than aborting the run, because a tool that crashes mid-write should still
// let the user see everything that arrived before the failure.
func readEvents(r io.Reader, fn func(Event), text func(string)) (stats eventStats) {
	sc := bufio.NewScanner(r)
	// Event data can be large (a finding with full evidence, a list of
	// endpoints), so raise the line limit well past bufio's 64KiB default.
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		// A JSON-shaped line is an envelope or a defect; anything else is the
		// tool's own output.
		if !strings.HasPrefix(line, "{") {
			stats.Text++
			if text != nil {
				text(visibleLine(line))
			}
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			stats.Malformed++
			continue
		}
		if ev.Event == "" {
			stats.Malformed++
			continue
		}
		stats.Decoded++
		fn(ev)
	}
	if err := sc.Err(); err != nil {
		// A truncated final line is normal when a run is cancelled: the
		// process is killed mid-write. It is not a reason to fail the run.
		stats.Err = err
	}
	return stats
}

type eventStats struct {
	Decoded   int
	Malformed int
	// Text counts lines of the tool's own output rather than events.
	Text int
	Err  error
}

// visibleLine reduces a tool-printed line to its final on-screen state.
//
// A tool livens its progress reading with in-place redraws: "\rProbing 10%",
// then "\rProbing 37%", then "\rProbing 100%". To a reader that splits lines
// on newlines those rewrites are one line full of carriage returns. Collapsing
// to the last segment is exactly what the terminal showed, so the transcript
// reads like the run it recorded instead of a dump of control characters.
func visibleLine(s string) string {
	if !strings.ContainsRune(s, '\r') {
		return s
	}
	last := ""
	for _, part := range strings.Split(s, "\r") {
		if part != "" {
			last = part
		}
	}
	return strings.TrimRight(last, " \t")
}

// sortedKeys gives deterministic iteration over an event's data map, which
// JSON does not guarantee. Without this the detail panel reshuffles its rows
// on every render, which reads as flicker.
func sortedKeys(m map[string]any) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// dataString reads a string-ish field, rendering numbers and booleans rather
// than dropping them. Event payloads are tool-authored and inconsistently
// typed; being forgiving here beats rendering an empty value.
func dataString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		// JSON numbers arrive as float64; render integers without a
		// fractional part so counts read as "14", not "14.000000".
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%g", t)
	case bool:
		return fmt.Sprintf("%t", t)
	case nil:
		return ""
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprintf("%v", t)
		}
		return string(b)
	}
}

// humanEvent turns an event type into a presentable label. It falls back to
// the raw type for unrecognised events so nothing is ever silently dropped.
func humanEvent(name string) string {
	if label, ok := eventLabels[name]; ok {
		return label
	}
	// "some.thing.happened" -> "Something happened", so an event type the
	// TUI has never seen still reads as English rather than as a token.
	parts := strings.Split(name, ".")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

var eventLabels = map[string]string{
	EventExecutionStarted:   "Execution started",
	EventExecutionCompleted: "Execution completed",
	EventExecutionFailed:    "Execution failed",
	EventExecutionCancelled: "Execution cancelled",
	EventFindingDiscovered:  "Finding",
	EventProgressUpdated:    "Progress",
	EventArtifactCreated:    "Artifact",
	EventTargetDiscovered:   "Target",
	"scan.started":          "Scan started",
	"scan.completed":        "Scan completed",
	"crash.detected":        "Crash",
	"anomaly.detected":      "Anomaly",
	"hang.detected":         "Hang",
	"input.minimized":       "Input minimized",
	"report.generated":      "Report",
	"evidence.collected":    "Evidence",
	"coverage.updated":      "Coverage",
	"behavior.discovered":   "Behavior",
	"corpus.added":          "Corpus",
	"corpus.trimmed":        "Corpus",
	"baseline.started":      "Baseline started",
	"baseline.completed":    "Baseline completed",
	"seed.created":          "Seed",
	"seed.executed":         "Seed executed",
	"campaign.started":      "Campaign started",
	"campaign.completed":    "Campaign completed",
	"module.loaded":         "Module",
	"target.resolved":       "Target resolved",
	"phase.started":         "Phase started",
	"phase.completed":       "Phase completed",
	"stage.started":         "Stage started",
	"stage.completed":       "Stage completed",
}
