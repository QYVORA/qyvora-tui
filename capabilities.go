package tui

import (
	"sort"
	"strings"
)

// Capabilities describes what a tool can do, as data.
//
// This is a normalised view over the capability registries the tools already
// maintain. Ten of the thirteen tools expose a `capabilities -o json` command
// backed by their own `internal/capabilities` package, but those registries
// predate this type and are not uniform: four distinct shapes are in the fleet,
// two tools expose none at all, and one (jabari) describes device support
// rather than runnable operations.
//
// Rather than a fifth schema, or forcing every tool to rewrite a registry that
// already works, this type is the union of what the existing registries
// express. Only ID and Name are required; a tool populates what it knows and
// leaves the rest absent. A registry that carries a field nobody modelled here
// keeps it in Extra rather than losing it.
type Capabilities struct {
	// Tool is the tool's own name, matching its event `framework` field.
	Tool string

	// Items are the individual capabilities, in registry order.
	Items []Capability

	// Family names the registry shape the entries were read from. It is
	// diagnostic only: the interface must never branch on it, because that
	// would reintroduce per-tool knowledge into the shared layer.
	Family string

	// Extra holds top-level registry fields this type does not model.
	Extra map[string]any
}

// Capability is one operation, transport, or provider a tool offers.
//
// The type is named Capability rather than Operation because the registries
// disagree on what they enumerate: sekhmet lists things you can run, jabari
// lists transports and binaries it can use. An interface that assumed every
// entry were runnable would offer jabari's USB transport as something to
// execute.
type Capability struct {
	// ID is the stable identifier, conventionally "tool.verb.noun". It is the
	// only field a tool must supply and the key everything else matches on.
	ID string

	// Name is the human label.
	Name string

	// Description is the one-line summary. It may legitimately be empty.
	Description string

	// Category groups related entries. Tools disagree on granularity, so it is
	// presented as a label and never interpreted.
	Category string

	// Risk is the tool's own grade, e.g. "S2". Kept as a string because the
	// fleet has no shared risk scale; normalising it here would invent a
	// meaning the tools do not agree on.
	Risk string

	// AuthorizationRequired records that the tool wants its authorisation flag
	// confirmed. Surfaced in the interface because a capability that acts on a
	// target is the one a user should read twice.
	AuthorizationRequired bool

	// ConfirmationRequired records that the tool will ask again at run time.
	ConfirmationRequired bool

	// Reversible reports whether the tool can undo the operation. HasReversible
	// separates "no" from "not stated": a capability whose registry omits
	// reversibility is not the same as one known not to be reversible.
	Reversible    bool
	HasReversible bool

	// ChangesState reports that the operation alters the target or host.
	ChangesState    bool
	HasChangesState bool

	// Live requires a live cloud provider. Only the cloud-oriented registry
	// family models this.
	Live    bool
	HasLive bool

	// Implemented separates a modelled capability that works from one the
	// registry lists as intended but has not built.
	Implemented    bool
	HasImplemented bool

	// Binary names an external program the capability needs.
	Binary string

	// Status is a tool-supplied availability label, such as jabari's
	// native/integration/missing. Preserved verbatim.
	Status string

	// TargetTypes are the kinds of target accepted, when the registry says so.
	TargetTypes []string

	// Produces lists the output schema types the capability can yield.
	Produces []string

	// ExpectedDuration is the tool's rough estimate, shown as a hint only and
	// never as a progress figure: a fabricated percentage is worse than none.
	ExpectedDuration string

	// Input is the capability's parameters, when the registry models them.
	//
	// Only the mansa/nzinga family does. A tool with no parameter metadata
	// leaves this empty, and the interface must not invent a form for it.
	Input []Parameter

	// Topics are the event types this capability emits, when the registry
	// exposes them. They let the interface say what to expect before a run
	// without the shared TUI knowing any tool's vocabulary.
	Topics []string

	// Extra holds registry fields this type does not model.
	Extra map[string]any
}

// Parameter is one input to a capability, as the tool's registry describes it.
type Parameter struct {
	Name        string
	Type        string
	Required    bool
	Description string
	Default     string
	// Choices are the accepted values, when the registry constrains them.
	Choices []string
}

// HasInput reports whether the capability describes parameters. The interface
// uses this to decide whether a structured form is possible, rather than
// assuming one exists.
func (c Capability) HasInput() bool { return len(c.Input) > 0 }

// IsImplemented reports whether the capability is usable.
//
// A registry that does not model implementation status is taken at its word:
// listing a capability is a claim that it works. Modelling the field and saying
// false is a real negative.
func (c Capability) IsImplemented() bool {
	if c.HasImplemented {
		return c.Implemented
	}
	return true
}

// RequiresAuthorization reports whether acting on this capability touches a
// target, and so deserves the operator's attention before it runs.
func (c Capability) RequiresAuthorization() bool {
	return c.AuthorizationRequired || c.ChangesState
}

// Entry is one row of the capability region: enough to identify a capability
// and show the facts that matter before running it.
type Entry struct {
	ID     string
	Name   string
	Detail string
	// Available is false for a capability the registry reports as
	// unimplemented or as missing a binary it needs.
	Available bool
	// Note explains an unavailable row, so it can say why rather than merely
	// being dimmed.
	Note string
}

// Entries reduces the capabilities to display rows.
//
// Registry order is preserved rather than sorted: it is the tool's own
// ordering, it groups related capabilities together, and a tool that chose an
// order should not have it lost by a presentation layer.
func (c *Capabilities) Entries() []Entry {
	if c == nil {
		return nil
	}
	out := make([]Entry, 0, len(c.Items))
	for _, e := range c.Items {
		row := Entry{
			ID:        e.ID,
			Name:      e.Name,
			Available: e.IsImplemented() && e.Binary == "",
		}
		// The note leads with the tool's own status word when it gave one.
		// "missing" is jabari's word, and jabari is the authority on whether
		// frida is present; restating it in the TUI's phrasing would lose that.
		switch {
		case !e.IsImplemented():
			row.Note = "not implemented"
		case e.Status == "missing":
			row.Note = "missing"
		case e.Binary != "":
			row.Note = "needs " + e.Binary
		case e.Status != "" && e.Status != "native":
			row.Note = e.Status
		case e.Live:
			row.Note = "live provider"
		}
		row.Detail = e.Detail(0)
		out = append(out, row)
	}
	return out
}

// Detail renders the one-line summary shown beside a capability, truncated to
// width when width is positive.
func (c Capability) Detail(width int) string {
	var b strings.Builder
	add := func(s string) {
		if s == "" {
			return
		}
		if b.Len() > 0 {
			b.WriteString("  ")
		}
		b.WriteString(s)
	}
	if c.Risk != "" {
		add("risk " + c.Risk)
	}
	if c.RequiresAuthorization() {
		add("authorised target")
	}
	if c.Live {
		add("live provider")
	}
	if c.ExpectedDuration != "" {
		add(c.ExpectedDuration)
	}
	out := b.String()
	if width > 1 && len(out) > width {
		return out[:width-1] + "…"
	}
	return out
}

// Categories returns the distinct categories present, sorted, so navigation
// does not reshuffle between renders.
func (c *Capabilities) Categories() []string {
	if c == nil {
		return nil
	}
	seen := make(map[string]bool, len(c.Items))
	out := make([]string, 0, len(c.Items))
	for _, e := range c.Items {
		if e.Category == "" || seen[e.Category] {
			continue
		}
		seen[e.Category] = true
		out = append(out, e.Category)
	}
	sort.Strings(out)
	return out
}

// Find returns the capability with the given ID.
//
// Matching tries the full dotted ID first, then a trailing segment, so
// `campaign.fuzz` finds `sekhmet.campaign.fuzz`. The tolerance is deliberate: a
// user who has heard a capability named by its short form should not have to
// also recall the framework prefix.
func (c *Capabilities) Find(id string) (Capability, bool) {
	if c == nil {
		return Capability{}, false
	}
	for _, e := range c.Items {
		if e.ID == id {
			return e, true
		}
	}
	for _, e := range c.Items {
		if strings.HasSuffix(e.ID, "."+id) {
			return e, true
		}
	}
	return Capability{}, false
}

// Formable returns the capabilities that describe their own parameters, and so
// could drive a structured form.
//
// This is the gate that keeps the interface honest: a tool whose registry
// models no inputs yields an empty list, and the form layer then stays out of
// the way instead of inventing fields a scanner never asked for.
func (c *Capabilities) Formable() []Capability {
	if c == nil {
		return nil
	}
	var out []Capability
	for _, e := range c.Items {
		if e.HasInput() {
			out = append(out, e)
		}
	}
	return out
}

// lifecycleNames are the event topics the whole fleet agrees on.
//
// Seven topics appear in all twelve event-emitting tools, and the measured
// distribution is bimodal: these, then a long tail of tool-specific topics with
// almost nothing in between. Treating exactly this set as universal keeps the
// Activity region honest. Widening it to look more capable would assert
// agreement the data does not show.
var lifecycleNames = map[string]bool{
	"scan.started":       true,
	"scan.completed":     true,
	"finding.discovered": true,
	"report.generated":   true,
	"error":              true,
	"warning":            true,
	"info":               true,
}

// stageNames are the near-universal progress topics. Ten of twelve tools emit
// them under two different names: the fleet split between `stage.*` and
// `phase.*`. Both are recognised so identical progress treatment applies either
// way, and the split stays visible in one place rather than scattered through
// the rendering code.
var stageNames = map[string]bool{
	"stage.started":   true,
	"stage.completed": true,
	"phase.started":   true,
	"phase.completed": true,
}

// IsLifecycle reports whether an event type is one of the universal topics.
func IsLifecycle(topic string) bool { return lifecycleNames[topic] }

// IsStage reports whether an event type is a stage or progress transition.
func IsStage(topic string) bool { return stageNames[topic] }
