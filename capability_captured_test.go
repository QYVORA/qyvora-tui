package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The files in testdata are captured verbatim from the tools' own
// `capabilities -o json` output, not written by hand for these tests. That is
// the whole point of normalising the existing registries: if a tool's shape
// changes, this test must notice, and a hand-written fixture would only prove
// the test agrees with itself.
//
// Regenerate with, for each tool: `./<tool> capabilities -o json > testdata/<tool>.json`
var capturedRegistries = []struct {
	tool    string
	file    string
	wantIDs []string
}{
	// Family A: id/name/description/framework/category/output_schema/risk/
	// authorization_required/target_type/confirmation_required/reversible.
	{tool: "sekhmet", file: "sekhmet.json", wantIDs: []string{
		"sekhmet.baseline.profile", "sekhmet.campaign.fuzz",
	}},
	// Family C: family A plus changes_state and target_types, with parameters
	// nested under a schema object.
	{tool: "mansa", file: "mansa.json", wantIDs: []string{"mansa.analyze"}},
	// Family B: a document wrapper with a capabilities list, a commands list and
	// tool-level metadata.
	{tool: "kush", file: "kush.json", wantIDs: []string{"kush.intake"}},
	// Family D: name/category/status/description, no id at all.
	{tool: "jabari", file: "jabari.json", wantIDs: []string{"Device discovery"}},
}

func readRegistry(t *testing.T, file string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", file))
	if err != nil {
		t.Fatalf("reading %s: %v", file, err)
	}
	return b
}

func TestNormalizeCapturedRegistries(t *testing.T) {
	for _, tc := range capturedRegistries {
		t.Run(tc.tool, func(t *testing.T) {
			c, err := NormalizeCapabilities(tc.tool, readRegistry(t, tc.file))
			if err != nil {
				t.Fatalf("normalising captured %s output: %v", tc.tool, err)
			}
			if len(c.Items) == 0 {
				t.Fatal("normalised to zero capabilities")
			}
			if c.Family == "" {
				t.Error("Family not recorded")
			}
			for _, want := range tc.wantIDs {
				if _, ok := c.Find(want); !ok {
					t.Errorf("captured %s output did not yield %q", tc.tool, want)
				}
			}
			// Every entry must be able to name itself, or the interface has a
			// blank row it cannot label.
			for i, e := range c.Items {
				if e.Name == "" {
					t.Errorf("entry %d has no name (id %q)", i, e.ID)
				}
			}
			if got := c.Entries(); len(got) != len(c.Items) {
				t.Errorf("Entries() produced %d rows for %d capabilities", len(got), len(c.Items))
			}
		})
	}
}

func TestCapturedSekhmetRegistry(t *testing.T) {
	c, err := NormalizeCapabilities("sekhmet", readRegistry(t, "sekhmet.json"))
	if err != nil {
		t.Fatal(err)
	}
	prof, ok := c.Find("sekhmet.baseline.profile")
	if !ok {
		t.Fatal("baseline.profile not found")
	}
	if prof.Risk != "S1" {
		t.Errorf("Risk = %q, want S1", prof.Risk)
	}
	if !prof.AuthorizationRequired {
		t.Error("authorization_required lost")
	}
	if !prof.HasReversible || !prof.Reversible {
		t.Error("reversible lost")
	}
	if got := prof.Produces; len(got) != 1 || got[0] != "Baseline" {
		t.Errorf("Produces = %v, want [Baseline]", got)
	}
	if got := prof.TargetTypes; len(got) != 1 || got[0] != "any" {
		t.Errorf("TargetTypes = %v, want [any] -- target_type is a bare string in this registry", got)
	}
}

func TestCapturedMansaParameters(t *testing.T) {
	c, err := NormalizeCapabilities("mansa", readRegistry(t, "mansa.json"))
	if err != nil {
		t.Fatal(err)
	}
	analyze, ok := c.Find("mansa.analyze")
	if !ok {
		t.Fatal("mansa.analyze not found")
	}
	// The parameters are one level down, under schema.input. Reading them is the
	// only reason a form is possible for mansa at all.
	if !analyze.HasInput() {
		t.Fatal("no parameters read from the nested schema")
	}
	first := analyze.Input[0]
	if first.Name != "session" || first.Type != "string" || !first.Required {
		t.Errorf("first parameter = %+v, want the required session string", first)
	}
	// The output half of the schema describes results, not inputs. It must not
	// be offered as something to fill in.
	for _, p := range analyze.Input {
		if p.Name == "findings" {
			t.Error("an output description was read as an input parameter")
		}
	}
	if len(analyze.Produces) == 0 {
		t.Error("output list lost")
	}
	if !analyze.HasChangesState {
		t.Error("HasChangesState not set even though the registry states it")
	}
	if got := c.Formable(); len(got) == 0 {
		t.Error("Formable() is empty, so mansa could never get a form")
	}
}

func TestCapturedKushDocumentMetadata(t *testing.T) {
	c, err := NormalizeCapabilities("kush", readRegistry(t, "kush.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := c.Extra["framework"]; !ok || got != "kush" {
		t.Errorf("Extra[framework] = %v (present %t)", got, ok)
	}
	if got, ok := c.Extra["version"]; !ok || got == "" {
		t.Error("version was dropped from the document")
	}
	if got, ok := c.Extra["exit_codes"]; !ok || got == nil {
		t.Error("exit_codes was dropped from the document")
	}
	if got, ok := c.Extra["event_verbs"]; !ok || got == nil {
		t.Error("event_verbs was dropped; the interface can use them to say what to expect")
	}
	// The document's own command list is a container, not a property of the
	// tool, so it is not preserved as top-level metadata.
	if _, ok := c.Extra["commands"]; ok {
		t.Error("the commands container was kept as tool metadata")
	}
	intake, ok := c.Find("kush.intake")
	if !ok {
		t.Fatal("kush.intake not found")
	}
	if !intake.IsImplemented() {
		t.Error("IsImplemented() = false for a capability the registry marks implemented")
	}
	if !intake.HasLive {
		t.Error("HasLive not set even though the registry states live: false")
	}
	if intake.Live {
		t.Error("Live = true for a capability the registry marks live: false")
	}
}

func TestCapturedJabariStatusRows(t *testing.T) {
	c, err := NormalizeCapabilities("jabari", readRegistry(t, "jabari.json"))
	if err != nil {
		t.Fatal(err)
	}
	rows := c.Entries()
	if len(rows) != 21 {
		t.Fatalf("got %d rows, want the 21 captured entries", len(rows))
	}
	// jabari has no id field, so the name is the identity. Without that the
	// entries could not be referred to at all.
	for i, r := range rows {
		if r.ID == "" {
			t.Errorf("row %d has no identity", i)
		}
	}
	// The registry states a status for every entry, and the interface has to
	// carry it: a native capability and a missing one are not the same row, and
	// jabari is the only authority on which is which.
	native, missing, needsBinary := 0, 0, 0
	for i, r := range rows {
		switch r.Note {
		case "":
			native++
		case "missing":
			missing++
		case "needs adb":
			// The one integration the registry states *and* a binary for. The
			// binary is the actionable half, so it is what the row names.
			needsBinary++
		default:
			t.Errorf("row %d (%s) has note %q", i, r.Name, r.Note)
		}
		if r.Available != (r.Note == "") {
			t.Errorf("row %d (%s): Available=%t but note %q", i, r.Name, r.Available, r.Note)
		}
	}
	if native != 16 {
		t.Errorf("native = %d, want 16", native)
	}
	if missing != 4 {
		t.Errorf("missing = %d, want the 4 optional integrations the registry reports", missing)
	}
	if needsBinary != 1 {
		t.Errorf("needsBinary = %d, want 1", needsBinary)
	}
	usb, ok := c.Find("USB transport")
	if !ok {
		t.Fatal("USB transport not found by name")
	}
	if usb.Status != "integration" {
		t.Errorf("Status = %q, want integration", usb.Status)
	}
	// The USB transport needs adb, so it is shown as a requirement rather than
	// silently dimmed: the tool can use it, and it says what it needs.
	if row := c.Entries()[2]; row.Available || !strings.HasPrefix(row.Note, "needs ") {
		t.Errorf("USB transport row = %+v, want unavailable with a note naming adb", row)
	}
	// A jabari entry is a device capability, not a runnable operation, so the
	// form gate must not treat it as one.
	if got := c.Formable(); len(got) != 0 {
		t.Errorf("Formable() returned %d entries for a registry with no parameters", len(got))
	}
}

// fleetCapability mirrors the JSON shape one of the fleet's registries prints,
// declared here as a Go value rather than a fixture. It is the shape the tools
// pass to CapabilitiesFrom, and it is declared independently of the normaliser
// so a change to either has to be made deliberately.
type fleetCapability struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Risk       string `json:"risk"`
	AuthNeeded bool   `json:"authorization_required"`
	Reversible bool   `json:"reversible"`
}

func TestCapabilitiesFromTheToolsOwnList(t *testing.T) {
	// The discovery path: a tool hands over the exact value its own
	// `capabilities -o json` marshals. One registry, read two ways.
	list := []fleetCapability{
		{ID: "probe.scan.tcp", Name: "TCP port scan", Risk: "S1", AuthNeeded: true, Reversible: true},
		{ID: "probe.scan.udp", Name: "UDP port scan", Risk: "S2", AuthNeeded: true},
	}
	c, err := CapabilitiesFrom("probe", list)
	if err != nil {
		t.Fatalf("CapabilitiesFrom: %v", err)
	}
	if len(c.Items) != 2 {
		t.Fatalf("got %d capabilities, want 2", len(c.Items))
	}
	first := c.Items[0]
	if first.ID != "probe.scan.tcp" || first.Name != "TCP port scan" || first.Risk != "S1" {
		t.Errorf("first = %+v", first)
	}
	if !first.RequiresAuthorization() {
		t.Error("the authorisation flag was lost")
	}
	if !first.HasReversible || !first.Reversible {
		t.Error("reversibility was not read")
	}
	// The Go field has no omitempty, so the second entry states reversible:false
	// explicitly. "Stated as no" is different from "not stated", and the
	// normaliser has to keep them apart.
	second := c.Items[1]
	if !second.HasReversible || second.Reversible {
		t.Errorf("a stated false was not recorded as stated: %+v", second)
	}
}

func TestCapabilitiesFromRejectsAnUnencodableList(t *testing.T) {
	// A tool that cannot produce its own registry has a problem worth surfacing,
	// not one to paper over with an empty interface.
	if _, err := CapabilitiesFrom("probe", map[string]any{"ch": make(chan int)}); err == nil {
		t.Fatal("expected an error for an unencodable list")
	}
}

func TestCapabilitiesFromAnEmptyList(t *testing.T) {
	// A tool with an empty but valid registry normalises to nothing, which the
	// interface renders as no region at all.
	c, err := CapabilitiesFrom("probe", []fleetCapability{})
	if err != nil {
		t.Fatalf("an empty list failed: %v", err)
	}
	if len(c.Items) != 0 {
		t.Errorf("got %d items, want 0", len(c.Items))
	}
}
