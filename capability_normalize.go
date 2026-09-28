package tui

import (
	"encoding/json"
	"fmt"
	"strings"
)

// capabilityFamilies maps the registry shapes found across the fleet onto the
// normalised field names.
//
// Four shapes are in use, and they are not variations of one schema:
//
//	sekhmet, shaka                       id/name/description/framework/category/
//	                                      output_schema/risk/authorization_required/
//	                                      target_type/confirmation_required/reversible
//	imhotep, amanirenas, kush,           Document wrapper + Capability{id,name,
//	sundiata, timbuktu                   implemented,live,note} + provider flags
//	mansa, nzinga                        the above, plus parameters and
//	                                      changes_state/target_types/duration
//	jabari                               name/category/status/description/binary
//
// Rather than a fifth schema or a rewrite of working registries, each shape
// contributes a field map and the rules for reading it live in one place. A new
// shape is then one more entry here, and the interface still never learns a
// tool's particular vocabulary.
var capabilityFamilies = []capabilityFamily{
	{
		// A tool record carrying the sekhmet/shaka field set. This is the
		// richest operation schema, and also the one mansa/nzinga extend.
		Name:    "operations",
		Array:   true,
		ID:      []string{"id", "ID", "capability_id", "command"},
		Command: true,
		// Every runnable-operation record in the fleet carries an id. That is
		// the field that separates this shape from the name-only device shape,
		// which also has a "name" but no identity.
		Match: [][]string{{"id", "name"}},
		Fields: map[string]capField{
			"id":                     {name: "id", list: true},
			"capability_id":          {name: "id", list: true},
			"name":                   {name: "name", list: true},
			"summary":                {name: "name", list: true},
			"description":            {name: "description", list: true},
			"category":               {name: "category", list: true},
			"framework":              {name: "framework", list: true},
			"risk":                   {name: "risk", list: true},
			"authorization_required": {name: "authorization_required", bool: true, list: true},
			"auth_required":          {name: "authorization_required", bool: true, list: true},
			"confirmation_required":  {name: "confirmation_required", bool: true, list: true},
			"note":                   {name: "description", list: true},
			"target_type":            {name: "target_types", strings: true, list: true},
			"target_types":           {name: "target_types", strings: true, list: true},
			"expected_duration":      {name: "expected_duration", list: true},
			"duration":               {name: "expected_duration", list: true},
			"reversible":             {name: "reversible", bool: true, list: true},
			"changes_state":          {name: "changes_state", bool: true, list: true},
			"output_schema":          {name: "output_schema", strings: true, list: true},
			"output":                 {name: "output_schema", strings: true, list: true},
			"input":                  {name: "input", params: true, list: true},
			"parameters":             {name: "input", params: true, list: true},
			// mansa nests the parameter list one level down, under a schema
			// object, alongside a list of output descriptions. Only the input
			// half describes how to run the capability; the output half is a
			// description of results and is kept in Extra.
			"schema":       {name: "", nested: true},
			"topics":       {name: "topics", strings: true, list: true},
			"events":       {name: "topics", strings: true, list: true},
			"capability":   {name: "", nested: true},
			"capabilities": {name: "", list: true},
			"commands":     {name: "", list: true},
			"implemented":  {name: "implemented", bool: true, list: true},
			"live":         {name: "live", bool: true, list: true},
			"binary":       {name: "binary", list: true},
			"status":       {name: "status", list: true},
		},
	},
	{
		// The jabari shape: a list of named device capabilities with an
		// availability status rather than a set of runnable operations.
		Name:  "devices",
		Array: true,
		ID:    []string{"id", "name"},
		// The device shape is name-keyed and states an availability status.
		Match: [][]string{{"name", "status"}, {"name", "binary"}},
		Fields: map[string]capField{
			"name":        {name: "name", list: true},
			"category":    {name: "category", list: true},
			"status":      {name: "status", list: true},
			"description": {name: "description", list: true},
			"binary":      {name: "binary", list: true},
			"capability":  {name: "name", list: true},
			"implemented": {name: "implemented", bool: true, list: true},
		},
	},
}

// capField describes how one registry key reaches the normalised type.
type capField struct {
	// name is the destination on Capability. Empty means the key is a
	// container rather than a value.
	name string
	// bool coerces a value to bool and records that it was stated, which is how
	// "no" is kept distinct from "not reported".
	bool bool
	// strings turns a value into a []string, accepting a bare string.
	strings bool
	// params reads a parameter list.
	params bool
	// nested descends into a sub-object.
	nested bool
	// list accepts either a single object or an array of them.
	list bool
}

// capabilityFamily is one registry shape.
type capabilityFamily struct {
	// Name is diagnostic only. The interface must never branch on it; that
	// would put per-tool knowledge back into the shared layer.
	Name string
	// Array reports whether the registry's top level is a list of records.
	Array bool
	// ID is the candidate key order for an entry's identity.
	ID []string
	// Command reports whether this family is derived from the command registry
	// rather than a separate capability table.
	Command bool
	// Match lists field combinations that identify the shape. A family claims a
	// registry only when a record actually carries one of them, which is what
	// keeps two shapes that both use "name" from being read as each other.
	Match [][]string
	// Fields maps registry keys onto the normalised type.
	Fields map[string]capField
	// DocFields covers a document wrapper, where the list sits beside
	// tool-level metadata.
	DocFields map[string]capField
}

// NormalizeCapabilities reads a tool's own `capabilities -o json` output.
//
// The bytes are the tool's existing registry, not something the TUI authored,
// so there is no second source of truth to drift: the same command the operator
// can run by hand is what the interface reads. Any field the fleet's shapes do
// not model is preserved in Extra rather than dropped.
//
// A registry the TUI cannot interpret is an error, not an empty list. Silently
// showing "no capabilities" for a tool that has them would be worse than
// saying the registry could not be read.
func NormalizeCapabilities(tool string, data []byte) (*Capabilities, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, fmt.Errorf("%s: empty capability registry", tool)
	}
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%s: capability registry is not JSON: %w", tool, err)
	}
	// A tool that publishes no capabilities is a real state, not a failure:
	// aksum and anansi have no capability table, and toha3ee's registry is its
	// own command set. Report that honestly and let the interface adapt.
	if recs, ok := records(raw); ok && len(recs) == 0 {
		return &Capabilities{Tool: tool}, nil
	}

	recs, ok := records(raw)
	if !ok {
		return nil, fmt.Errorf("%s: capability registry has no capability list", tool)
	}
	for _, fam := range capabilityFamilies {
		if !fam.matches(recs) {
			continue
		}
		caps := &Capabilities{Tool: tool, Family: fam.Name, Extra: map[string]any{}}
		caps.Items = fam.read(recs)
		// A document-shaped registry also carries tool-level metadata beside
		// the capability list -- framework, version, output modes. It is kept
		// rather than discarded, because the registry is the tool's own record
		// of itself and a normaliser that drops the parts it does not model
		// would make the interface quietly less informed than the data.
		if doc, isDoc := raw.(map[string]any); isDoc {
			for key, val := range fam.docExtra(doc) {
				caps.Extra[key] = val
			}
		}
		return caps, nil
	}
	return nil, fmt.Errorf("%s: capability registry matches no known shape", tool)
}

// records finds the list of capability records in a decoded registry.
//
// It handles both observed layouts: a bare array of records, and an object
// wrapping one under a conventional key.
func records(raw any) ([]map[string]any, bool) {
	switch v := raw.(type) {
	case []any:
		out := make([]map[string]any, 0, len(v))
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out, true
	case map[string]any:
		for _, key := range []string{"capabilities", "capability", "commands", "tools"} {
			if arr, ok := v[key].([]any); ok {
				return records(any(arr))
			}
		}
	}
	return nil, false
}

// containerKeys are the document keys that hold the capability list rather than
// describing the tool.
var containerKeys = map[string]bool{
	"capabilities": true,
	"capability":   true,
	"commands":     true,
	"tools":        true,
}

// docExtra returns the document-level fields that describe the tool itself.
//
// They are returned verbatim rather than flattened onto the capabilities: the
// interface is allowed to show them, but no capability may claim a property
// that belongs to the tool as a whole. A per-record "framework" would
// otherwise read as a per-capability one and mean something different.
func (f capabilityFamily) docExtra(doc map[string]any) map[string]any {
	out := make(map[string]any, len(doc))
	for key, raw := range doc {
		if containerKeys[key] {
			continue
		}
		out[key] = raw
	}
	return out
}

// matches reports whether a family can read these records.
//
// A family claims a registry when it recognises the fields it needs. Requiring
// a minimum rather than any overlap keeps a sparse registry from being read by
// the wrong shape: a record with only a name and a description is not
// evidence of a shape.
func (f capabilityFamily) matches(recs []map[string]any) bool {
	if len(recs) == 0 || len(f.Match) == 0 {
		return false
	}
	// Every record has to look like this shape, not just the first. A registry
	// is homogeneous by construction, so one outlier means the shape is wrong
	// rather than that the record set should be read in two passes.
	for _, rec := range recs {
		claimed := false
		for _, combo := range f.Match {
			if hasAll(rec, combo) {
				claimed = true
				break
			}
		}
		if !claimed {
			return false
		}
	}
	return true
}

// hasAll reports whether a record carries every key in a combination.
func hasAll(rec map[string]any, keys []string) bool {
	for _, k := range keys {
		if _, ok := rec[k]; !ok {
			return false
		}
	}
	return len(keys) > 0
}

// read normalises a set of records.
func (f capabilityFamily) read(recs []map[string]any) []Capability {
	out := make([]Capability, 0, len(recs))
	for _, rec := range recs {
		if c, ok := f.readOne(rec); ok {
			out = append(out, c)
		}
	}
	return out
}

// readOne normalises a single record.
func (f capabilityFamily) readOne(rec map[string]any) (Capability, bool) {
	c := Capability{Extra: map[string]any{}}
	for key, raw := range rec {
		fld, ok := f.Fields[key]
		if !ok {
			// Not modelled. Keep it, so a tool's registry stays authoritative
			// and nothing it publishes is silently lost.
			c.Extra[key] = raw
			continue
		}
		f.assign(&c, fld, raw)
	}
	// A nested schema contributes its input list to the parameters, and is
	// otherwise kept whole in Extra. The output half of a schema describes
	// results rather than how to run the capability, so folding it into Input
	// would make the interface offer the user fields to fill in that are in
	// fact its output.
	if len(c.Input) == 0 {
		if schema, ok := rec["schema"].(map[string]any); ok {
			c.Input = append(c.Input, readParams(schema["input"])...)
			for k, v := range schema {
				if k != "input" {
					c.Extra[k] = v
				}
			}
		}
	}

	// A nested capability wrapper (family B puts the real record one level
	// down) is unwrapped here rather than in each field, so the rest of the
	// mapping stays flat.
	if inner, ok := rec["capability"].(map[string]any); ok {
		if c.ID == "" || c.Name == "" {
			sub := f
			sub.Fields = f.Fields
			if inner2, ok := sub.readOne(inner); ok {
				if c.ID == "" {
					c.ID = inner2.ID
				}
				if c.Name == "" {
					c.Name = inner2.Name
				}
				if c.Description == "" {
					c.Description = inner2.Description
				}
				for k, v := range inner2.Extra {
					if _, exists := c.Extra[k]; !exists {
						c.Extra[k] = v
					}
				}
			}
		}
	}
	if c.Name == "" && c.ID == "" {
		return Capability{}, false
	}
	if c.ID == "" {
		// A registry entry with only a name still deserves a stable key, so the
		// interface can refer to it.
		c.ID = c.Name
	}
	if c.Name == "" {
		c.Name = c.ID
	}
	return c, true
}

// assign writes one registry field onto the normalised capability.
func (f capabilityFamily) assign(c *Capability, fld capField, raw any) {
	switch {
	case fld.nested:
		// A container; readOne handles the unwrapping.
		return
	case fld.params:
		c.Input = append(c.Input, readParams(raw)...)
	case fld.strings:
		if s := toStrings(raw); len(s) > 0 {
			setField(c, fld.name, s)
		}
	case fld.bool:
		if b, ok := raw.(bool); ok {
			setField(c, fld.name, b)
		}
	default:
		if s := toString(raw); s != "" {
			setField(c, fld.name, s)
		}
	}
}

// setField assigns a normalised field, recording presence for the booleans
// where "no" and "not reported" must stay distinguishable.
func setField(c *Capability, name string, v any) {
	switch name {
	case "id":
		c.ID = v.(string)
	case "topics":
		c.Topics = v.([]string)
	case "name":
		c.Name = v.(string)
	case "description":
		c.Description = v.(string)
	case "category":
		c.Category = v.(string)
	case "framework":
		// The framework repeats on every record of a family-A/B/C registry. It
		// is kept in Extra rather than hoisted onto the capability: which
		// framework a record names is a property of the record, and the
		// tool-level value lives on the registry itself.
		c.Extra["framework"] = v
	case "risk":
		c.Risk = v.(string)
	case "binary":
		c.Binary = v.(string)
	case "status":
		c.Status = v.(string)
	case "expected_duration":
		c.ExpectedDuration = v.(string)
	case "authorization_required":
		c.AuthorizationRequired = v.(bool)
	case "confirmation_required":
		c.ConfirmationRequired = v.(bool)
	case "reversible":
		c.Reversible = v.(bool)
		c.HasReversible = true
	case "changes_state":
		c.ChangesState = v.(bool)
		c.HasChangesState = true
	case "implemented":
		c.Implemented = v.(bool)
		c.HasImplemented = true
	case "live":
		c.Live = v.(bool)
		c.HasLive = true
	case "target_types":
		c.TargetTypes = v.([]string)
	case "output_schema":
		c.Produces = v.([]string)
	}
}

// readParams normalises a parameter list.
//
// Only the mansa/nzinga family publishes these today. A tool with no parameter
// metadata yields no parameters, and the form layer then stays out of the way
// instead of inventing fields the operation never asked for.
func readParams(raw any) []Parameter {
	arr, ok := raw.([]any)
	if !ok {
		if m, ok := raw.(map[string]any); ok {
			// A single parameter, or a map of name to definition.
			if _, has := m["name"]; has {
				return []Parameter{readParam(m)}
			}
			out := make([]Parameter, 0, len(m))
			for name, def := range m {
				d, ok := def.(map[string]any)
				if !ok {
					out = append(out, Parameter{Name: name})
					continue
				}
				if d["name"] == nil {
					d["name"] = name
				}
				out = append(out, readParam(d))
			}
			return out
		}
		return nil
	}
	out := make([]Parameter, 0, len(arr))
	for _, item := range arr {
		if m, ok := item.(map[string]any); ok {
			out = append(out, readParam(m))
		}
	}
	return out
}

// readParam normalises one parameter definition.
func readParam(m map[string]any) Parameter {
	p := Parameter{
		Name:        toString(m["name"]),
		Type:        toString(m["type"]),
		Description: toString(m["description"]),
		Default:     toString(m["default"]),
	}
	if b, ok := m["required"].(bool); ok {
		p.Required = b
	}
	p.Choices = toStrings(m["choices"])
	if p.Choices == nil {
		p.Choices = toStrings(m["enum"])
	}
	return p
}

// toString renders a JSON scalar, tolerating the numeric and boolean values a
// registry may use for a label.
func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%g", t)
	case bool:
		return fmt.Sprintf("%t", t)
	default:
		return ""
	}
}

// toStrings normalises a value that may be a single string or a list of them.
func toStrings(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s := toString(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	default:
		return nil
	}
}

// CapabilitiesFrom normalises a capability list the tool already holds in
// memory.
//
// This is the discovery path the tools use. A tool's `capabilities -o json`
// output is produced by marshalling the same value this takes, so the interface
// and the machine-readable contract cannot disagree: there is one registry, read
// two ways. That is the property a second hand-maintained list would break.
//
// The value is marshalled rather than reflected over, so a tool is free to pass
// the exact type its own CLI prints. A marshalling failure is returned rather
// than swallowed, because a tool that cannot produce its own registry has a
// problem worth surfacing.
func CapabilitiesFrom(tool string, list any) (*Capabilities, error) {
	data, err := json.Marshal(list)
	if err != nil {
		return nil, fmt.Errorf("%s: capability list cannot be encoded: %w", tool, err)
	}
	return NormalizeCapabilities(tool, data)
}
