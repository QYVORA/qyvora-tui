package tui

import (
	"testing"
)

// The four registry shapes below are transcribed from the tools' real
// `capabilities -o json` output. They are not invented: each is the field set
// measured in the fleet, so a change to a tool's registry that breaks
// normalisation shows up here rather than in the interface.

const familyAJSON = `[
  {"id":"sekhmet.baseline.profile","name":"Profile target baseline",
   "description":"Measure the target's normal behaviour.","framework":"sekhmet",
   "category":"baseline","output_schema":["Baseline"],"risk":"S1",
   "authorization_required":true,"target_type":"any",
   "confirmation_required":false,"reversible":true,"expected_duration":"minutes"},
  {"id":"sekhmet.campaign.fuzz","name":"Run a fuzzing campaign",
   "description":"Run a mutation campaign.","framework":"sekhmet",
   "category":"fuzzing","output_schema":["SessionResult","Finding","Crash"],
   "risk":"S2","authorization_required":true,"target_type":"any",
   "confirmation_required":false,"reversible":true}
]`

const familyBJSON = `{
  "name":"kush","framework":"kush","version":"0.4.0",
  "output_modes":["terminal","json"],"exit_codes":true,"sim_support":false,
  "capabilities":[
    {"id":"kush.static.analyze","name":"Static analysis","implemented":true,"live":false},
    {"id":"kush.cloud.submit","name":"Cloud submission","implemented":true,"live":true}
  ],
  "commands":[{"name":"analyze","implemented":true}]
}`

const familyCJSON = `[
  {"id":"mansa.wifi.discover","name":"Discover wireless interfaces",
   "description":"Enumerate wireless interfaces.","framework":"mansa",
   "category":"discovery","output":["Interface"],"risk":"S1",
   "authorization_required":true,"confirmation_required":false,
   "reversible":true,"changes_state":true,"target_types":["any"],"duration":"seconds",
   "input":[{"name":"interface","type":"string","required":true,"description":"iface"},
            {"name":"duration","type":"int","required":false,"default":"30"}]}
]`

const familyDJSON = `[
  {"name":"Device discovery","category":"Android Engine","status":"native",
   "description":"USB/TCP device discovery"},
  {"name":"USB transport","category":"Android Engine","status":"integration",
   "description":"ADB protocol over USB","binary":"adb"}
]`

func TestNormalizeFamilyAOperations(t *testing.T) {
	c := normalize(t, "sekhmet", familyAJSON)
	if len(c.Items) != 2 {
		t.Fatalf("got %d capabilities, want 2", len(c.Items))
	}
	first := c.Items[0]
	if first.ID != "sekhmet.baseline.profile" {
		t.Errorf("ID = %q", first.ID)
	}
	if first.Name != "Profile target baseline" {
		t.Errorf("Name = %q", first.Name)
	}
	if first.Risk != "S1" {
		t.Errorf("Risk = %q, want S1", first.Risk)
	}
	if !first.AuthorizationRequired {
		t.Error("AuthorizationRequired = false, want true")
	}
	if !first.HasReversible || !first.Reversible {
		t.Error("reversible not recorded as stated-and-true")
	}
	if got := first.TargetTypes; len(got) != 1 || got[0] != "any" {
		t.Errorf("TargetTypes = %v", got)
	}
	if got := first.Produces; len(got) != 1 || got[0] != "Baseline" {
		t.Errorf("Produces = %v, want [Baseline]", got)
	}
	if first.ExpectedDuration != "minutes" {
		t.Errorf("ExpectedDuration = %q", first.ExpectedDuration)
	}
	if !first.RequiresAuthorization() {
		t.Error("RequiresAuthorization() = false for an authorised capability")
	}
}

func TestNormalizeFamilyBDocumentWrapper(t *testing.T) {
	c := normalize(t, "kush", familyBJSON)
	if len(c.Items) != 2 {
		t.Fatalf("got %d capabilities, want 2", len(c.Items))
	}
	live := c.Items[1]
	if !live.HasLive || !live.Live {
		t.Error("live flag not recorded for the cloud capability")
	}
	if !live.IsImplemented() {
		t.Error("IsImplemented() = false, want true")
	}
	if live.Name != "Cloud submission" {
		t.Errorf("Name = %q", live.Name)
	}
	// The family-B registry states the framework once, at document level. It
	// belongs to the tool, not to any one capability, so it is kept on the
	// registry and must not be hoisted onto the entries.
	if got, ok := c.Extra["framework"]; !ok || got != "kush" {
		t.Errorf("document framework = %v (present %t), want kush", got, ok)
	}
	if _, ok := live.Extra["framework"]; ok {
		t.Error("a tool-level property was copied onto a single capability")
	}
	if got := c.Extra["output_modes"]; got == nil {
		t.Error("output_modes was dropped from the document")
	}
}

func TestNormalizeFamilyCParameters(t *testing.T) {
	c := normalize(t, "mansa", familyCJSON)
	if len(c.Items) != 1 {
		t.Fatalf("got %d capabilities, want 1", len(c.Items))
	}
	cap0 := c.Items[0]
	if !cap0.HasChangesState || !cap0.ChangesState {
		t.Error("changes_state not recorded")
	}
	if !cap0.HasInput() {
		t.Fatal("HasInput() = false, want the two declared parameters")
	}
	if len(cap0.Input) != 2 {
		t.Fatalf("got %d parameters, want 2", len(cap0.Input))
	}
	first := cap0.Input[0]
	if first.Name != "interface" || first.Type != "string" || !first.Required {
		t.Errorf("first parameter = %+v", first)
	}
	if cap0.Input[1].Default != "30" {
		t.Errorf("default not read: %+v", cap0.Input[1])
	}
}

func TestNormalizeFamilyDDeviceStatus(t *testing.T) {
	c := normalize(t, "jabari", familyDJSON)
	if len(c.Items) != 2 {
		t.Fatalf("got %d capabilities, want 2", len(c.Items))
	}
	usb := c.Items[1]
	if usb.Status != "integration" {
		t.Errorf("Status = %q, want integration", usb.Status)
	}
	if usb.Binary != "adb" {
		t.Errorf("Binary = %q, want adb", usb.Binary)
	}
	// A capability needing an external binary is not available, and the row
	// must be able to say why.
	rows := c.Entries()
	if rows[1].Available {
		t.Error("a capability needing a binary was reported available")
	}
	if rows[1].Note != "needs adb" {
		t.Errorf("Note = %q, want %q", rows[1].Note, "needs adb")
	}
	if !rows[0].Available {
		t.Error("a native capability was reported unavailable")
	}
	if rows[0].Note != "" {
		t.Errorf("an available capability should carry no note, got %q", rows[0].Note)
	}
}

func TestNormalizeEmptyRegistryIsNotAnError(t *testing.T) {
	// aksum and anansi publish no capability registry. That is a real state,
	// not a failure, and the interface has to cope with it.
	c := normalize(t, "aksum", `[]`)
	if c == nil {
		t.Fatal("empty registry normalised to nil")
	}
	if len(c.Items) != 0 {
		t.Errorf("got %d items, want 0", len(c.Items))
	}
	if got := c.Entries(); len(got) != 0 {
		t.Errorf("Entries() = %v, want empty", got)
	}
	if got := c.Formable(); len(got) != 0 {
		t.Errorf("Formable() = %v, want empty", got)
	}
}

func TestNormalizeRejectsUnreadableRegistry(t *testing.T) {
	// A registry that cannot be read must say so. Returning an empty list would
	// tell the operator a tool has no capabilities when it publishes some.
	for _, tc := range []struct{ name, body string }{
		{"not json", `sekhmet: command not found`},
		{"empty", ``},
		{"unknown shape", `{"unrelated":{"nested":true}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NormalizeCapabilities("tool", []byte(tc.body)); err == nil {
				t.Fatal("expected an error, got nil")
			}
		})
	}
}

func TestNormalizePreservesUnmodelledFields(t *testing.T) {
	// A registry is authoritative. A field this type does not model must be
	// kept rather than dropped, or normalising would lose information the tool
	// deliberately published.
	body := `[{"id":"x.y","name":"Y","description":"d","a_field_from_the_future":42}]`
	c := normalize(t, "x", body)
	got, ok := c.Items[0].Extra["a_field_from_the_future"]
	if !ok {
		t.Fatal("unmodelled field was dropped")
	}
	if got != float64(42) {
		t.Errorf("value = %v, want 42", got)
	}
}

func TestCapabilitiesFindMatchesShortAndFullID(t *testing.T) {
	c := normalize(t, "sekhmet", familyAJSON)
	if _, ok := c.Find("sekhmet.campaign.fuzz"); !ok {
		t.Error("full ID did not resolve")
	}
	got, ok := c.Find("campaign.fuzz")
	if !ok {
		t.Fatal("short ID did not resolve")
	}
	if got.Name != "Run a fuzzing campaign" {
		t.Errorf("resolved the wrong entry: %q", got.Name)
	}
	if _, ok := c.Find("nope.nope"); ok {
		t.Error("an unknown ID resolved to an entry")
	}
	var nilCaps *Capabilities
	if _, ok := nilCaps.Find("anything"); ok {
		t.Error("nil registry resolved an entry")
	}
}

func TestIsLifecycleAndIsStageCoverTheFleet(t *testing.T) {
	// The seven universal topics and the stage/phase split are measured
	// properties of the fleet. If a future tool standardises on a different
	// name, this test is where it should surface.
	for _, topic := range []string{
		"scan.started", "scan.completed", "finding.discovered",
		"report.generated", "error", "warning", "info",
	} {
		if !IsLifecycle(topic) {
			t.Errorf("IsLifecycle(%q) = false, want true", topic)
		}
	}
	if IsLifecycle("crash.detected") {
		t.Error("a tool-specific topic was classified as universal")
	}
	// The fleet split 7 tools on stage.* and 2 on phase.*; both must be
	// recognised or two tools lose their progress treatment.
	for _, topic := range []string{"stage.started", "phase.started", "stage.completed", "phase.completed"} {
		if !IsStage(topic) {
			t.Errorf("IsStage(%q) = false, want true", topic)
		}
	}
	if IsStage("scan.started") {
		t.Error("scan.started is not a stage transition")
	}
}
