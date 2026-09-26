// Package field derives form and validation schemas from tagged Go structs,
// so a struct is the single source for its type, its checks, the UI form and
// the YAML contract.
package field

import (
	"fmt"
	"reflect"
	"strings"
)

// Type is how a UI edits a field.
type Type string

const (
	Number Type = "number"
	Text   Type = "string"
	Flag   Type = "boolean"
	List   Type = "list"
	Choice Type = "choice"
)

// Field describes one attribute or assumption. It is derived from struct tags,
// so the Go type, validation, the UI form and the YAML contract share one source.
//
//	MemorySize float64 `scout:"memory_size" label:"Memory" unit:"MB" default:"128"`
//
// A non-pointer field without a default is required. A pointer field is optional.
// `options:"a,b"` turns a string (or []string) into a choice.
// `path:"block.attr"` says where the value sits in the Terraform resource.
type Field struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Type     Type     `json:"type"`
	Unit     string   `json:"unit,omitempty"`
	Hint     string   `json:"hint,omitempty"`
	Default  any      `json:"default,omitempty"`
	Options  []string `json:"options,omitempty"`
	Multi    bool     `json:"multi,omitempty"`
	Required bool     `json:"required"`
	Path     string   `json:"path,omitempty"`

	index []int
}

// RefStep in a path follows a reference: "task_definition->cpu" is the cpu
// of the resource that the task_definition attribute references.
const RefStep = "->"

// TerraformPath is the dotted location of the value in a Terraform resource.
// "ref->path" reads path on the resource the attribute ref references, and a
// last step "#" counts blocks ("scratch_disk.#"); a step "*" reads the rest
// in every block as a list ("disk.*.disk_type").
func (f Field) TerraformPath() string {
	if f.Path != "" {
		return f.Path
	}
	return f.Key
}

// FieldsOf derives fields from a struct type. It panics on a malformed struct,
// which is a programming error caught by the registry test.
func FieldsOf(t reflect.Type) []Field {
	fields, err := fieldsOf(t)
	if err != nil {
		panic(err.Error())
	}
	return fields
}

func fieldsOf(t reflect.Type) ([]Field, error) {
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("FieldsOf: %s is not a struct", t)
	}
	var out []Field
	seen := map[string]bool{}
	for i := 0; i < t.NumField(); i++ {
		fields, err := fieldsAt(t, i)
		if err != nil {
			return nil, err
		}
		for _, f := range fields {
			if seen[f.Key] {
				return nil, fmt.Errorf("FieldsOf: %s: key %q is declared twice", t, f.Key)
			}
			seen[f.Key] = true
			out = append(out, f)
		}
	}
	return out, nil
}

// fieldsAt is what the i-th struct field of t contributes: its own field if
// tagged, the fields of an embedded struct, or nothing.
func fieldsAt(t reflect.Type, i int) ([]Field, error) {
	sf := t.Field(i)
	if sf.Anonymous && sf.Type.Kind() == reflect.Struct {
		// An embedded struct contributes its fields: this is how facets compose.
		inner, err := fieldsOf(sf.Type)
		for k := range inner {
			inner[k].index = append([]int{i}, inner[k].index...)
		}
		return inner, err
	}
	key := sf.Tag.Get("scout")
	if key == "" {
		return nil, nil
	}
	f, err := tagged(key, sf)
	if err != nil {
		return nil, fmt.Errorf("FieldsOf: %s.%s: %w", t, sf.Name, err)
	}
	f.index = []int{i}
	return []Field{f}, nil
}

// tagged builds the field that struct field sf declares under key.
func tagged(key string, sf reflect.StructField) (Field, error) {
	f := Field{
		Key:   key,
		Label: sf.Tag.Get("label"),
		Unit:  sf.Tag.Get("unit"),
		Hint:  sf.Tag.Get("hint"),
		Path:  sf.Tag.Get("path"),
	}
	if f.Label == "" {
		f.Label = key
	}
	if o := sf.Tag.Get("options"); o != "" {
		f.Options = strings.Split(o, ",")
	}
	ft := sf.Type
	optional := ft.Kind() == reflect.Pointer
	if optional {
		ft = ft.Elem()
	}
	var err error
	if f.Type, f.Multi, err = typeOf(ft, f.Options != nil); err != nil {
		return f, err
	}
	d, ok := sf.Tag.Lookup("default")
	if !ok {
		f.Required = !optional
		return f, nil
	}
	if optional {
		return f, fmt.Errorf("optional fields cannot have a default")
	}
	if f.Default, err = coerce(f, d); err != nil {
		return f, fmt.Errorf("bad default: %v", err)
	}
	return f, nil
}

// typeOf is how a UI edits a Go type; a string or []string with options is a choice.
func typeOf(ft reflect.Type, options bool) (typ Type, multi bool, err error) {
	switch ft.Kind() {
	case reflect.Float64, reflect.Int:
		return Number, false, nil
	case reflect.String:
		if options {
			return Choice, false, nil
		}
		return Text, false, nil
	case reflect.Bool:
		return Flag, false, nil
	case reflect.Slice:
		if ft.Elem().Kind() != reflect.String {
			return "", false, fmt.Errorf("only []string lists are supported")
		}
		if options {
			return Choice, true, nil
		}
		return List, false, nil
	}
	return "", false, fmt.Errorf("unsupported kind %s", ft.Kind())
}
