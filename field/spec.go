package field

import "fmt"

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
