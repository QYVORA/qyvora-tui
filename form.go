package tui

import (
	"strings"
	"unicode/utf8"
)

// Form is a structured input for one capability, built from the parameter
// metadata the tool's own registry publishes.
//
// The gate is the whole design. OpenForm returns nil for a capability that
// describes no parameters, and the interface then keeps its composer. A form
// for a tool that publishes no input schema would be the TUI inventing fields
// an operator has to guess at, which is worse than typing arguments.
//
// Nothing here knows any tool's vocabulary. A field is a name, a type and a
// required flag, all read from the registry; the form's job is to check that
// what the operator typed satisfies what the tool said, not to decide what the
// tool accepts.
type Form struct {
	// Capability is the capability this form fills in.
	Capability Capability

	fields []*formField
	// focus is the index of the field being edited.
	focus int

	// err is set by the last failed submit, and is shown on the form rather
	// than as a notice, so it stays on screen while it is being fixed.
	err string
}

// formField is one input.
type formField struct {
	param  Parameter
	value  string
	cursor int
}

// OpenForm builds a form for a capability, or nil when the capability describes
// no parameters.
//
// The nil return is load-bearing: callers treat it as "keep the composer".
func OpenForm(c Capability) *Form {
	if !c.HasInput() {
		return nil
	}
	f := &Form{Capability: c}
	// A default is pre-filled rather than left blank. The tool said what the
	// value is when it is unset, and typing it in for the operator is the
	// difference between a form and a chore.
	for _, p := range c.Input {
		f.fields = append(f.fields, &formField{param: p, value: p.Default, cursor: len(p.Default)})
	}
	return f
}

// Len reports how many fields the form has.
func (f *Form) Len() int {
	if f == nil {
		return 0
	}
	return len(f.fields)
}

// Focus returns the field being edited, or nil.
func (f *Form) Focus() *formField {
	if f == nil || f.focus < 0 || f.focus >= len(f.fields) {
		return nil
	}
	return f.fields[f.focus]
}

// Err returns the last validation message.
func (f *Form) Err() string {
	if f == nil {
		return ""
	}
	return f.err
}

// FocusNext and FocusPrev move between fields. They report whether the focus
// moved, so a caller can leave the form when it runs off the end rather than
// silently wrapping: a form that wraps is a form you cannot leave with the
// arrow keys.
func (f *Form) FocusNext() bool {
	if f == nil || f.focus+1 >= len(f.fields) {
		return false
	}
	f.focus++
	f.err = ""
	return true
}

func (f *Form) FocusPrev() bool {
	if f == nil || f.focus == 0 {
		return false
	}
	f.focus--
	f.err = ""
	return true
}

// Insert adds a rune at the cursor.
//
// The cursor is a byte offset into the value, because that is what slicing
// needs, but it always lands on a rune boundary: a value is typed a character
// at a time and moved a character at a time, so an offset that can fall inside a
// multi-byte character is an offset that will eventually produce mojibake.
func (f *Form) Insert(r rune) {
	fl := f.Focus()
	if fl == nil {
		return
	}
	fl.value = fl.value[:fl.cursor] + string(r) + fl.value[fl.cursor:]
	fl.cursor = len(fl.value[:fl.cursor] + string(r))
}

// Backspace deletes the rune before the cursor.
func (f *Form) Backspace() {
	fl := f.Focus()
	if fl == nil || fl.cursor == 0 {
		return
	}
	_, size := utf8.DecodeLastRuneInString(fl.value[:fl.cursor])
	if size == 0 {
		return
	}
	fl.value = fl.value[:fl.cursor-size] + fl.value[fl.cursor:]
	fl.cursor -= size
}

// MoveCursor shifts the cursor within the focused field, and reports whether it
// moved.
//
// A field is edited in place, so the cursor has to be able to go both ways. A
// form that can only append cannot change a character in the middle of a value,
// which is most of editing.
func (f *Form) MoveCursor(delta int) bool {
	fl := f.Focus()
	if fl == nil || delta == 0 {
		return false
	}
	cur := fl.cursor
	for ; delta < 0 && cur > 0; delta++ {
		_, size := utf8.DecodeLastRuneInString(fl.value[:cur])
		cur -= size
	}
	for ; delta > 0 && cur < len(fl.value); delta-- {
		_, size := utf8.DecodeRuneInString(fl.value[cur:])
		cur += size
	}
	if cur == fl.cursor {
		return false
	}
	fl.cursor = cur
	return true
}

// MoveToStart and MoveToEnd put the cursor at either end of the focused field.
func (f *Form) MoveToStart() {
	if fl := f.Focus(); fl != nil {
		f.MoveCursor(-fl.cursor)
	}
}

func (f *Form) MoveToEnd() {
	if fl := f.Focus(); fl != nil {
		f.MoveCursor(len(fl.value) - fl.cursor)
	}
}

// ClearField empties the focused field, which is what a dedicated delete key
// should do to a field rather than to the character before the cursor.
func (f *Form) ClearField() {
	fl := f.Focus()
	if fl == nil {
		return
	}
	fl.value = ""
	fl.cursor = 0
}

// Args renders the filled-in form as command-line arguments.
//
// A field with no value is omitted entirely rather than passed as an empty
// string, because `--duration ""` is a value the tool has to reject and an
// omitted flag is not. That rule is the one judgement call here, and it errs
// towards sending less: an argument the operator did not fill in is not
// something the interface should invent.
func (f *Form) Args() []string {
	if f == nil {
		return nil
	}
	var out []string
	for _, fl := range f.fields {
		if fl.value == "" {
			continue
		}
		out = append(out, "--"+fl.param.Name, fl.value)
	}
	return out
}

// fail records a message the form could not proceed with.
//
// It is the same channel as a validation error and is shown the same way: on
// the form, next to the thing being fixed, rather than as a notice that scrolls
// away.
func (f *Form) fail(msg string) {
	if f == nil {
		return
	}
	f.err = msg
}

// Validate reports whether the form is complete enough to run.
//
// Only the required flag is checked. Type coercion is the tool's job: the
// registry says a value is an int, and the tool is where an int is parsed, so
// duplicating that here would give two parsers to disagree.
func (f *Form) Validate() error {
	if f == nil {
		return errNoForm
	}
	for _, fl := range f.fields {
		if fl.param.Required && fl.value == "" {
			f.err = fl.param.Name + " is required"
			return &formError{f.err}
		}
	}
	// A choice the registry constrains is checked here rather than by the tool,
	// because the accepted set is data the registry published and nothing else
	// knows.
	for _, fl := range f.fields {
		if err := allowed(fl); err != nil {
			f.err = err.Error()
			return err
		}
	}
	f.err = ""
	return nil
}

// allowed reports whether a field's value is one the registry accepts.
//
// A field with no choices constrains nothing, and an empty value is the
// operator declining to choose rather than choosing wrongly, so both pass.
func allowed(fl *formField) error {
	if len(fl.param.Choices) == 0 || fl.value == "" {
		return nil
	}
	for _, c := range fl.param.Choices {
		if c == fl.value {
			return nil
		}
	}
	return &formError{fl.param.Name + " must be one of " + strings.Join(fl.param.Choices, ", ")}
}

// errNoForm is returned when a form is used as though one were open.
var errNoForm = &formError{"no capability is open"}

// formError is a validation failure with no underlying cause.
//
// The message is duplicated on the form so the interface can show it next to
// the field it concerns, where a notice in the transcript would scroll away
// from the field being fixed.
type formError struct{ msg string }

func (e *formError) Error() string { return e.msg }
