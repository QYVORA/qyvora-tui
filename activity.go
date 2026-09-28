package tui

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// Activity is the live view of a run: what is happening, what has been
// discovered, and how much of it there is.
//
// It is a consumer of events and nothing else. It does not know what a scan
// is, does not interpret a tool's vocabulary, and holds no reference to any
// scanner. A tool it has never seen produces a perfectly good Activity
// region, because the only thing it requires is the seven-field envelope every
// tool already emits.
//
// The design consequence is deliberate: the measured topic distribution is
// bimodal -- seven topics shared by all twelve tools, then a long tail of
// tool-specific ones. So this tracks counts and the most recent lines, and
// treats every topic alike. Anything cleverer would have to guess which
// tool-specific topic is important, and a wrong guess invents a priority the
// tool never expressed.
type Activity struct {
	// Total is every event seen for the current run.
	Total int

	// ByLevel counts events by the envelope's level field. Levels the TUI does
	// not know about are still counted, under their own name, so a tool using
	// "notice" does not have its events vanish from the tally.
	ByLevel map[string]int

	// Recent holds the tail of event labels, newest last, for the region to
	// show. It is a ring: a long run must not grow the interface's memory
	// without bound.
	Recent []string

	// Stages holds the current stage and the most recently completed one, for
	// tools that emit stage transitions.
	CurrentStage string
	LastStage    string

	// Findings counts finding events, which is the one universal topic that
	// reliably means "the tool produced something worth reading".
	Findings int

	// Started is when the current run began.
	Started time.Time
}

// activityTail is how many recent events the region shows. Enough to read a
// run's shape, few enough that a 200-line terminal still shows a dozen.
const activityTail = 12

// newActivity starts an activity view for a run.
func newActivity() *Activity {
	return &Activity{ByLevel: map[string]int{}, Started: time.Now()}
}

// record folds one event into the activity view.
//
// Every event is counted and, where there is room, described. An event whose
// topic the TUI has never seen is treated exactly like one it has: unknown is
// not an error, and dropping it would discard exactly the information the
// tool took the trouble to emit.
func (a *Activity) record(ev Event) {
	if a == nil {
		return
	}
	a.Total++
	level := ev.Level
	if level == "" {
		level = "info"
	}
	a.ByLevel[level]++
	a.Note(ev.Event)

	if IsStage(ev.Event) {
		name := dataString(ev.Data, "name")
		if name == "" {
			name = dataString(ev.Data, "stage")
		}
		if name == "" {
			name = dataString(ev.Data, "phase")
		}
		if name != "" {
			if strings.HasSuffix(ev.Event, ".started") {
				a.CurrentStage = name
			} else {
				a.LastStage = name
				a.CurrentStage = ""
			}
		}
	}
	if ev.Event == EventFindingDiscovered {
		a.Findings++
	}
}

// Note records a topic in the recent tail without incrementing counts. The
// region's own status and header lines go through here so the tail is a
// consistent record rather than a mix of two sources.
func (a *Activity) Note(label string) {
	if a == nil || label == "" {
		return
	}
	a.Recent = append(a.Recent, label)
	if len(a.Recent) > activityTail {
		a.Recent = a.Recent[len(a.Recent)-activityTail:]
	}
}

// Elapsed reports how long the run has been going.
func (a *Activity) Elapsed() time.Duration {
	if a == nil || a.Started.IsZero() {
		return 0
	}
	return time.Since(a.Started)
}

// levelSeverity is the order levels are shown in, most urgent first.
//
// One ranking serves the region, the error check and the styling, so a level
// cannot be urgent in one place and ordinary in another.
//
// A level that is not listed is not treated as ordinary information. It is
// ranked last and styled as muted detail, because a level the TUI has never
// heard of is one whose urgency it cannot claim to know: showing it is right,
// calling it an error would be a guess, and styling it as routine information
// would quietly imply the same.
func levelSeverity(level string) int {
	switch strings.ToLower(level) {
	case "fatal", "critical":
		return 0
	case "error":
		return 1
	case "warning", "warn":
		return 2
	case "info", "notice":
		return 3
	case "debug", "trace":
		return 4
	default:
		return 5
	}
}

// isErrorLevel reports whether a level counts as a failure.
func isErrorLevel(level string) bool { return levelSeverity(level) <= 1 }

// levelsBySeverity returns the levels this run actually used, most urgent
// first.
//
// The list comes from the events rather than from a fixed vocabulary, because a
// tool that emits `warning` or `notice` should not have those events counted
// and then left out of the display. Within a severity the order is stable, so
// two renders of the same run show the same rows in the same places.
func (a *Activity) levelsBySeverity() []string {
	if a == nil {
		return nil
	}
	levels := make([]string, 0, len(a.ByLevel))
	for level := range a.ByLevel {
		if a.ByLevel[level] > 0 {
			levels = append(levels, level)
		}
	}
	sort.Slice(levels, func(i, j int) bool {
		si, sj := levelSeverity(levels[i]), levelSeverity(levels[j])
		if si != sj {
			return si < sj
		}
		return levels[i] < levels[j]
	})
	return levels
}

// hasErrors reports whether anything went wrong, which is the one fact the
// region should lead with.
func (a *Activity) hasErrors() bool {
	if a == nil {
		return false
	}
	for level, n := range a.ByLevel {
		if n > 0 && isErrorLevel(level) {
			return true
		}
	}
	return false
}

// levelSymbol returns the glyph for a level.
//
// Symbols are chosen to match the status vocabulary the rest of the interface
// already uses, so a level means the same thing wherever it appears. The
// accompanying text is always present too: a symbol alone is not legible to
// everyone, and colour alone is not legible to anyone on a monochrome
// terminal.
func levelSymbol(level string) string {
	switch levelSeverity(level) {
	case 0, 1:
		return "✗"
	case 2:
		return "!"
	case 4:
		return "·"
	case 5:
		return "○"
	default:
		return "●"
	}
}

// Region renders the activity as a side region.
//
// The region leads with the counts, because "how much has happened" is the
// first question about a running command, and then lists the recent tail. It
// never claims a percentage: a run that emits no progress data has no
// percentage, and inventing one is the failure mode §39 warns about.
func (a *Activity) Region(t Theme, width int) *Region {
	r := NewRegion("ACTIVITY", width)
	r.Empty = "no activity"
	if a == nil || a.Total == 0 {
		return r
	}

	// The summary lines are the region's reason to exist at a glance.
	if stage := a.stageLine(); stage != "" {
		r.Add(t.Value.Render(clampLine(stage, width)))
	}
	r.Add(t.Label.Render("events") + " " + t.Value.Render(strconv.Itoa(a.Total)))
	if a.Findings > 0 {
		r.Add(t.Label.Render("findings") + " " + t.Success.Render(strconv.Itoa(a.Findings)))
	}
	// Errors first, because that is what an operator is looking for. The order
	// is the level's own severity rather than an alphabetical one, and every
	// level the run actually used is listed: a fixed list of names would drop
	// the events a tool invented, which is exactly the class of event this
	// region exists to make visible.
	for _, level := range a.levelsBySeverity() {
		n := a.ByLevel[level]
		if n == 0 {
			continue
		}
		// A level with no count is not shown at all. A region listing
		// "critical 0" and "debug 0" is reporting on its own vocabulary rather
		// than on the run, and the zero rows are the ones that push the
		// interesting ones off the bottom.
		r.Add(Row{
			Marker: t.levelStyle(level).Render(levelSymbol(level)),
			Key:    t.levelStyle(level).Render(level),
			Value:  t.Value.Render(strconv.Itoa(n)),
		}.String(width))
	}
	if el := a.Elapsed(); el > 0 {
		r.Add(t.Detail.Render(clampLine("elapsed "+duration(el), width)))
	}
	if len(r.Rows) > 0 {
		r.Add("")
	}

	// The tail, oldest first, so the newest event is the last line and needs no
	// marker to be found.
	for _, label := range a.Recent {
		r.Add(t.Activity.Render(clampLine("  "+humanEvent(label), width)))
	}
	return r
}

// stageLine renders the stage transition, preferring the one in progress.
func (a *Activity) stageLine() string {
	switch {
	case a.CurrentStage != "":
		return "● " + a.CurrentStage
	case a.LastStage != "":
		return "✓ " + a.LastStage
	default:
		return ""
	}
}
