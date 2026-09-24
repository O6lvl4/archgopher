package field

import (
	"fmt"
	"sort"
	"strings"
)

// Spec is a field written in a resource definition (YAML) rather than as a
// struct tag. A field without a default is required unless marked optional.
type Spec struct {
	Key      string   `yaml:"key"`
	Label    string   `yaml:"label"`
	Type     Type     `yaml:"type"`
	Unit     string   `yaml:"unit"`
	Hint     string   `yaml:"hint"`
	Default  any      `yaml:"default"`
	Options  []string `yaml:"options"`
	Multi    bool     `yaml:"multi"`
	Optional bool     `yaml:"optional"`
	Path     string   `yaml:"path"`
}

// Build validates a spec and turns it into a Field.
func (s Spec) Build() (Field, error) {
	f := Field{Key: s.Key, Label: s.Label, Type: s.Type, Unit: s.Unit, Hint: s.Hint, Options: s.Options, Multi: s.Multi, Path: s.Path}
	if f.Key == "" {
		return f, fmt.Errorf("a field has no key")
	}
	if f.Label == "" {
		f.Label = f.Key
	}
	switch f.Type {
	case Number, Text, Flag, List:
	case Choice:
		if len(f.Options) == 0 {
			return f, fmt.Errorf("field %q: a choice needs options", f.Key)
		}
	default:
		return f, fmt.Errorf("field %q: unknown type %q", f.Key, f.Type)
	}
	if s.Default != nil {
		if s.Optional {
			return f, fmt.Errorf("field %q: optional fields cannot have a default", f.Key)
		}
		v, err := coerce(f, s.Default)
		if err != nil {
			return f, fmt.Errorf("field %q: bad default: %w", f.Key, err)
		}
		f.Default = v
	} else {
		f.Required = !s.Optional
	}
	return f, nil
}

// DecodeValues validates values against fields and returns them normalized:
// numbers as float64, text as string, flags as bool, lists as []string.
// Defaults are applied; optional fields that are not set are absent. Unknown
// keys are errors unless listed in allow.
func DecodeValues(fields []Field, values map[string]any, what string, allow ...string) (map[string]any, error) {
	out := map[string]any{}
	known := map[string]bool{}
	for _, a := range allow {
		known[a] = true
	}
	var problems []string
	for _, f := range fields {
		known[f.Key] = true
		raw, present := values[f.Key]
		if !present || raw == nil {
			switch {
			case f.Default != nil:
				raw = f.Default
			case f.Required:
				problems = append(problems, fmt.Sprintf("missing %s %q", what, f.Key))
				continue
			default:
				continue
			}
		}
		v, err := coerce(f, raw)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s %q: %v", what, f.Key, err))
			continue
		}
		out[f.Key] = v
	}
	var unknown []string
	for k := range values {
		if !known[k] {
			unknown = append(unknown, k)
		}
	}
	sort.Strings(unknown)
	for _, k := range unknown {
		problems = append(problems, fmt.Sprintf("unknown %s %q", what, k))
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return out, nil
}
