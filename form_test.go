package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// The form tests exist to protect one property: a form appears only where the
// tool's own registry describes inputs, and never invents fields otherwise.

func TestOpenFormRefusesACapabilityWithNoParameters(t *testing.T) {
	// This is the gate. A tool that publishes no input schema gets the composer,
	// which is the correct interface for it, rather than a form of invented
	// fields.
	for _, c := range []Capability{
		{},
		{ID: "sekhmet.campaign.fuzz", Name: "Run a fuzzing campaign"},
		{ID: "jabari.usb", Name: "USB transport", Input: nil},
	} {
		if f := OpenForm(c); f != nil {
			t.Errorf("OpenForm(%q) returned a form with %d fields", c.ID, f.Len())
		}
	}
	var nilForm *Form
	if nilForm.Len() != 0 || nilForm.Args() != nil || nilForm.Focus() != nil {
		t.Error("a nil form is not inert")
	}
	if nilForm.FocusNext() || nilForm.FocusPrev() {
		t.Error("a nil form moved focus")
	}
	nilForm.Insert('x')
	nilForm.Backspace()
	nilForm.ClearField()
	if err := nilForm.Validate(); err != errNoForm {
		t.Errorf("Validate on a nil form = %v", err)
	}
}

func TestFormUsesTheRegistrysOwnParameters(t *testing.T) {
	// mansa.analyze declares one required session parameter, nested under its
	// schema. The form is built from that and from nothing else.
	c, err := NormalizeCapabilities("mansa", readRegistry(t, "mansa.json"))
	if err != nil {
		t.Fatal(err)
	}
	cap0, ok := c.Find("mansa.analyze")
	if !ok {
		t.Fatal("mansa.analyze not found")
	}
	f := OpenForm(cap0)
	if f == nil {
		t.Fatal("a capability with parameters yielded no form")
	}
	if f.Len() != 1 {
		t.Fatalf("got %d fields, want the 1 the registry declares", f.Len())
	}
	if f.Focus().param.Name != "session" {
		t.Errorf("field = %q, want session", f.Focus().param.Name)
	}
	// The required flag comes from the registry, and it is enforced.
	if err := f.Validate(); err == nil {
		t.Fatal("an empty required field validated")
	}
	if !strings.Contains(f.Err(), "session") {
		t.Errorf("the error does not name the field: %q", f.Err())
	}
}

func TestFormEditing(t *testing.T) {
	f := OpenForm(Capability{ID: "x.y", Input: []Parameter{{Name: "iface"}}})
	for _, r := range "wlan0" {
		f.Insert(r)
	}
	if got := f.Args(); len(got) != 2 || got[0] != "--iface" || got[1] != "wlan0" {
		t.Errorf("Args() = %v, want [--iface wlan0]", got)
	}
	f.Backspace()
	if got := f.Args(); got[1] != "wlan" {
		t.Errorf("Args() = %v after backspace, want wlan", got)
	}
	// Backspace at the start of the field is not an error and does not eat the
	// character before the field.
	f.ClearField()
	f.Backspace()
	if got := f.Args(); len(got) != 0 {
		t.Errorf("an empty field produced arguments: %v", got)
	}
}

func TestFormEditsInTheMiddleOfAValue(t *testing.T) {
	// A field that can only be appended to cannot be corrected, which is most
	// of editing. The cursor moves both ways and inserts where it is.
	f := OpenForm(Capability{ID: "x.y", Input: []Parameter{{Name: "iface"}}})
	for _, r := range "wlan0" {
		f.Insert(r)
	}
	// The cursor sits after the last character; one step left puts it between
	// "wlan" and "0".
	if !f.MoveCursor(-1) {
		t.Fatal("the cursor did not move left")
	}
	f.Insert('9')
	if got := f.Args()[1]; got != "wlan90" {
		t.Errorf("inserted value = %q, want wlan90", got)
	}
	// Moving to the start and inserting puts the character first.
	f.MoveToStart()
	f.Insert('a')
	if got := f.Args()[1]; got != "awlan90" {
		t.Errorf("insert at the start = %q, want awlan90", got)
	}
	// The cursor is after the inserted 'a'; two steps right puts it between "awl"
	// and "an90", and backspace removes the character before it.
	for i := 0; i < 2; i++ {
		f.MoveCursor(1)
	}
	f.Backspace()
	if got := f.Args()[1]; got != "awan90" {
		t.Errorf("after backspace = %q, want awan90", got)
	}
	// Moving past either end is a no-op rather than a panic.
	f.MoveToStart()
	if f.MoveCursor(-100) {
		t.Error("moving left past the start reported a move")
	}
	f.MoveToEnd()
	if f.MoveCursor(100) {
		t.Error("moving right past the end reported a move")
	}
	if got := f.Args()[1]; got != "awan90" {
		t.Errorf("value changed while moving past the ends: %q", got)
	}
}

func TestFormEditingIsSafeForMultiByteCharacters(t *testing.T) {
	// A cursor that lands inside a multi-byte character produces mojibake the
	// moment anything is inserted or deleted there. Values are typed and moved a
	// character at a time, so the cursor has to move the same way.
	f := OpenForm(Capability{ID: "x.y", Input: []Parameter{{Name: "ssid"}}})
	for _, r := range "héllo" {
		f.Insert(r)
	}
	if got := f.Args()[1]; got != "héllo" {
		t.Fatalf("typed value = %q, want héllo", got)
	}
	// Walk the cursor across the é and confirm the value survives intact. "héllo"
	// is six bytes and five characters, so three steps left from the end lands
	// between the é and the first l.
	for i := 0; i < 3; i++ {
		f.MoveCursor(-1)
	}
	f.Insert('x')
	if got := f.Args()[1]; got != "héxllo" {
		t.Errorf("insert after a multi-byte rune = %q, want héxllo", got)
	}
	// Backspace from just before the x removes the whole é, not its first byte.
	f.MoveCursor(-1)
	f.Backspace()
	if got := f.Args()[1]; got != "hxllo" {
		t.Errorf("backspace over a multi-byte rune = %q, want hxllo", got)
	}
}

func TestFormPrefillsDefaults(t *testing.T) {
	// A default is the tool's own answer for the unset case, so it is filled in
	// rather than left as a blank to be typed.
	f := OpenForm(Capability{ID: "x.y", Input: []Parameter{{Name: "duration", Default: "30"}}})
	if err := f.Validate(); err != nil {
		t.Errorf("a defaulted required field did not validate: %v", err)
	}
	if got := f.Args(); len(got) != 2 || got[1] != "30" {
		t.Errorf("Args() = %v, want the default 30", got)
	}
}

func TestFormOmitsUnfilledOptionalFields(t *testing.T) {
	// An optional field left blank produces no argument at all. Sending
	// `--duration ""` would hand the tool a value it has to reject; omitting the
	// flag is what the operator meant.
	f := OpenForm(Capability{ID: "x.y", Input: []Parameter{
		{Name: "iface", Required: true},
		{Name: "duration"},
	}})
	for _, r := range "wlan0" {
		f.Insert(r)
	}
	if got := f.Args(); len(got) != 2 {
		t.Errorf("Args() = %v, want only the filled-in field", got)
	}
}

func TestFormFocusStopsAtTheEnds(t *testing.T) {
	f := OpenForm(Capability{ID: "x.y", Input: []Parameter{{Name: "a"}, {Name: "b"}}})
	if f.FocusPrev() {
		t.Error("focus moved before the first field")
	}
	if !f.FocusNext() {
		t.Error("focus did not move to the second field")
	}
	if f.Focus().param.Name != "b" {
		t.Errorf("focus is on %q, want b", f.Focus().param.Name)
	}
	if f.FocusNext() {
		t.Error("focus moved past the last field")
	}
	// Typing still lands in the field that has focus, not nowhere.
	f.Insert('z')
	if f.Focus().value != "z" {
		t.Errorf("value = %q, want z", f.Focus().value)
	}
}

func TestFormValidatesChoices(t *testing.T) {
	// A constrained field is checked here, because the accepted set is data the
	// registry published and nothing else knows.
	f := OpenForm(Capability{ID: "x.y", Input: []Parameter{
		{Name: "mode", Choices: []string{"fast", "safe"}},
	}})
	for _, r := range "wrong" {
		f.Insert(r)
	}
	if err := f.Validate(); err == nil {
		t.Fatal("a value outside the registry's choices validated")
	}
	if !strings.Contains(f.Err(), "fast") {
		t.Errorf("the error does not list the accepted values: %q", f.Err())
	}
	f.ClearField()
	for _, r := range "safe" {
		f.Insert(r)
	}
	if err := f.Validate(); err != nil {
		t.Errorf("a listed choice was rejected: %v", err)
	}
	// An empty constrained field is not an error: leaving it unset is the
	// operator declining to choose, not choosing wrongly.
	f.ClearField()
	if err := f.Validate(); err != nil {
		t.Errorf("an unset optional choice was rejected: %v", err)
	}
}

func TestFormClearsTheErrorOnNavigation(t *testing.T) {
	// A validation message that outlives the mistake it describes is worse than
	// none, so moving between fields clears it.
	f := OpenForm(Capability{ID: "x.y", Input: []Parameter{{Name: "a", Required: true}, {Name: "b"}}})
	f.Validate()
	if f.Err() == "" {
		t.Fatal("no error was recorded")
	}
	f.FocusNext()
	if f.Err() != "" {
		t.Errorf("the error survived a focus change: %q", f.Err())
	}
}

func TestFormOpenedFromTheCommandLine(t *testing.T) {
	// The end-to-end path: an operator types `form <id>`, and the capability is
	// resolved through the normalised registry, short form included. mansa is
	// used because it is the family that publishes parameters.
	caps := normalizeBytes(t, "mansa", "mansa.json")
	m := modelFor(t, 200, 40, caps)
	m.input.SetValue("form analyze")
	m.submit()
	if m.form == nil {
		t.Fatalf("no form opened: %v", m.notices)
	}
	if m.form.Capability.ID != "mansa.analyze" {
		t.Errorf("opened the wrong capability: %q", m.form.Capability.ID)
	}
	if m.form.Len() != 1 {
		t.Errorf("the form has %d fields, want the 1 the registry declares", m.form.Len())
	}
}

func TestFormRefusesACapabilityThatTakesNoParameters(t *testing.T) {
	// sekhmet's registry publishes no parameter metadata, so there is nothing to
	// fill in. The honest answer is a notice, not an empty form.
	caps := normalize(t, "sekhmet", familyAJSON)
	m := modelFor(t, 200, 40, caps)
	m.input.SetValue("form campaign.fuzz")
	m.submit()
	if m.form != nil {
		t.Error("a form opened for a capability that takes no parameters")
	}
	if len(m.notices) == 0 {
		t.Error("no notice explained the refusal")
	}
}

func TestFormRefusesAnUnknownCapability(t *testing.T) {
	caps := normalize(t, "sekhmet", familyAJSON)
	m := modelFor(t, 200, 40, caps)
	m.input.SetValue("form not.a.capability")
	m.submit()
	if m.form != nil {
		t.Error("a form opened for a capability that does not exist")
	}
	if len(m.notices) == 0 {
		t.Error("no notice explained the refusal")
	}
}

func TestFormNeedsARegistry(t *testing.T) {
	// aksum and anansi publish none. Asking for a form then has to say so
	// rather than silently doing nothing.
	m := modelFor(t, 200, 40, nil)
	m.input.SetValue("form anything")
	m.submit()
	if m.form != nil {
		t.Error("a form opened with no registry")
	}
	if len(m.notices) == 0 {
		t.Error("no notice explained that there is no registry")
	}
}

func TestFormTakesTheKeyboardWhileOpen(t *testing.T) {
	// A form the operator cannot type into because another key handled the
	// keystroke is a form that cannot be used.
	caps := normalizeBytes(t, "mansa", "mansa.json")
	m := modelFor(t, 200, 40, caps)
	m.input.SetValue("form mansa.analyze")
	m.submit()
	if m.form == nil {
		t.Fatalf("no form opened: %v", m.notices)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("sessionA")})
	m = updated.(model)
	if got := m.form.Focus().value; got != "sessionA" {
		t.Errorf("typed value = %q, want sessionA", got)
	}
	// The composer is not receiving them at the same time.
	if m.input.Value() != "" {
		t.Errorf("the composer also took the keystrokes: %q", m.input.Value())
	}
	// The form replaces the composer, and is drawn.
	out := stripANSI(m.View())
	if !strings.Contains(out, "FILL IN") || !strings.Contains(out, "sessionA") {
		t.Errorf("the form is not drawn:\\n%s", out)
	}
	// Enter runs the capability with the filled-in arguments, under the command
	// word the tree actually has rather than the registry's identifier.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.form != nil {
		t.Error("the form stayed open after submit")
	}
	if !m.running {
		t.Error("submitting the form did not start a run")
	}
	if b := m.blockByID(m.activeID); b == nil {
		t.Error("no block for the submitted form")
	} else if got := strings.Join(b.args, " "); got != "analyze --session sessionA" {
		t.Errorf("ran %q, want the real command word", got)
	}
}

// helpOf returns the help lines for a model using the new help renderer.
func helpOf(m model) []string {
	return GetHelpLines("", m.runner.Commands(), m.cfg.Title, m.width-4, &m.theme)
}

func TestHelpMentionsFormsOnlyWhereTheyWork(t *testing.T) {
	// The new help system per PROMPT1.md §4 focuses on commands, not TUI features.
	// Form and keyboard shortcuts are TUI features, not commands, so they aren't
	// in the command help anymore. This test now verifies the help index structure.
	caps := normalizeBytes(t, "mansa", "mansa.json")
	withCaps := stripANSI(strings.Join(helpOf(modelFor(t, 200, 40, caps)), "\n"))
	
	// Should have the standard help structure
	if !strings.Contains(withCaps, "COMMANDS") {
		t.Errorf("help should have COMMANDS header:\n%s", withCaps)
	}
	if !strings.Contains(withCaps, "Built-ins") {
		t.Errorf("help should have Built-ins group:\n%s", withCaps)
	}
	if !strings.Contains(withCaps, "help") {
		t.Errorf("help should list the help command:\n%s", withCaps)
	}

	// Should work with or without caps
	noCaps := stripANSI(strings.Join(helpOf(modelFor(t, 200, 40, nil)), "\n"))
	if !strings.Contains(noCaps, "COMMANDS") {
		t.Errorf("help should work without caps:\n%s", noCaps)
	}
}

func TestFormResolvesTheCommandWordFromTheLiveTree(t *testing.T) {
	// A capability ID is a registry identifier, not a command word. mansa
	// publishes "mansa.analyze" and its tree has "analyze", so submitting the ID
	// verbatim would ask for a command that does not exist.
	//
	// The resolution is by suffix, longest first, and it is the tool's own tree
	// that decides: a tree without the command yields no words, and the form says
	// so rather than running something the operator did not name.
	m := modelFor(t, 200, 40, nil)
	for _, tc := range []struct {
		id   string
		want []string
	}{
		{"mansa.analyze", []string{"analyze"}},
		{"analyze", []string{"analyze"}},
		{"sekhmet.campaign.fuzz", nil},
		{"mansa.session", []string{"session"}},
		{"unknown.thing", nil},
		{"", nil},
	} {
		if got := m.commandWordsFor(Capability{ID: tc.id}); !equalStrings(got, tc.want) {
			t.Errorf("commandWordsFor(%q) = %v, want %v", tc.id, got, tc.want)
		}
	}
}

func TestFormRefusesToRunACapabilityWithNoCommand(t *testing.T) {
	// Nothing is run, nothing is invented, and the form stays open so the
	// operator can read why and type the command by hand.
	m := modelFor(t, 200, 40, nil)
	m.form = OpenForm(Capability{
		ID:    "sekhmet.campaign.fuzz",
		Input: []Parameter{{Name: "iterations"}},
	})
	for _, r := range "10" {
		m.form.Insert(r)
	}
	if cmd := m.submitForm(); cmd != nil {
		t.Error("a form with no matching command started a run")
	}
	if m.running {
		t.Error("the run started")
	}
	if m.form == nil {
		t.Error("the form closed instead of explaining itself")
	}
	if out := stripANSI(m.View()); !strings.Contains(out, "type the command") {
		t.Errorf("the reason is not shown:\n%s", out)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestFormEscapeCancelsWithoutRunning(t *testing.T) {
	caps := normalizeBytes(t, "mansa", "mansa.json")
	m := modelFor(t, 200, 40, caps)
	m.input.SetValue("form mansa.analyze")
	m.submit()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	if m.form != nil {
		t.Error("esc left the form open")
	}
	if m.running {
		t.Error("esc started a run")
	}
	if strings.Contains(stripANSI(m.View()), "FILL IN") {
		t.Error("the form is still drawn")
	}
}

func TestFormBlocksValidationErrors(t *testing.T) {
	// A form missing a required field does not run anything; it says why, next to
	// the form, so the message is on screen while it is being fixed.
	caps := normalizeBytes(t, "mansa", "mansa.json")
	m := modelFor(t, 200, 40, caps)
	m.input.SetValue("form mansa.analyze")
	m.submit()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.running {
		t.Fatal("an incomplete form started a run")
	}
	if m.form == nil {
		t.Fatal("the form closed despite failing validation")
	}
	if out := stripANSI(m.View()); !strings.Contains(out, "required") {
		t.Errorf("the reason is not shown:\\n%s", out)
	}
}

func TestFormIncompleteRunKeepsPartialOutput(t *testing.T) {
	// The form path must not bypass the execution guards: one run at a time,
	// and a notice rather than a silent no-op.
	caps := normalizeBytes(t, "mansa", "mansa.json")
	m := modelFor(t, 200, 40, caps)
	m.input.SetValue("form mansa.analyze")
	m.submit()
	for _, r := range "s" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(model)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if !m.running {
		t.Fatal("the form did not start a run")
	}
	// Opening a form while a run is in flight is refused, for the same reason a
	// second command is: the session can only be running one thing, and a form
	// is a way of starting one.
	m.input.SetValue("form mansa.analyze")
	m.submit()
	if m.form != nil {
		t.Error("a second form opened while a run was in flight")
	}
	if len(m.notices) == 0 {
		t.Error("no notice explained the refusal")
	}
}

// normalizeBytes normalises captured registry bytes, for the tests that use the
// real tool output rather than a hand-written shape.
func normalizeBytes(t *testing.T, tool, file string) *Capabilities {
	t.Helper()
	c, err := NormalizeCapabilities(tool, readRegistry(t, file))
	if err != nil {
		t.Fatalf("normalising %s: %v", tool, err)
	}
	return c
}
