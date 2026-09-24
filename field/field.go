// Package field derives form and validation schemas from tagged Go structs,
// so a struct is the single source for its type, its checks, the UI form and
// the YAML contract.
package field

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
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

// TerraformPath is the dotted location of the value in a Terraform resource.
func (f Field) TerraformPath() string {
	if f.Path != "" {
		return f.Path
	}
	return f.Key
}

// FieldsOf derives fields from a struct type. It panics on a malformed struct,
// which is a programming error caught by the registry test.
func FieldsOf(t reflect.Type) []Field {
	if t.Kind() != reflect.Struct {
		panic(fmt.Sprintf("FieldsOf: %s is not a struct", t))
	}
	var out []Field
	seen := map[string]bool{}
	add := func(f Field) {
		if seen[f.Key] {
			panic(fmt.Sprintf("FieldsOf: %s: key %q is declared twice", t, f.Key))
		}
		seen[f.Key] = true
		out = append(out, f)
	}
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.Anonymous && sf.Type.Kind() == reflect.Struct {
			// An embedded struct contributes its fields: this is how facets compose.
			for _, inner := range FieldsOf(sf.Type) {
				inner.index = append([]int{i}, inner.index...)
				add(inner)
			}
			continue
		}
		key := sf.Tag.Get("scout")
		if key == "" {
			continue
		}
		f := Field{
			Key:   key,
			Label: sf.Tag.Get("label"),
			Unit:  sf.Tag.Get("unit"),
			Hint:  sf.Tag.Get("hint"),
			Path:  sf.Tag.Get("path"),
			index: []int{i},
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
		switch ft.Kind() {
		case reflect.Float64, reflect.Int:
			f.Type = Number
		case reflect.String:
			f.Type = Text
			if f.Options != nil {
				f.Type = Choice
			}
		case reflect.Bool:
			f.Type = Flag
		case reflect.Slice:
			if ft.Elem().Kind() != reflect.String {
				panic(fmt.Sprintf("FieldsOf: %s.%s: only []string lists are supported", t, sf.Name))
			}
			f.Type = List
			if f.Options != nil {
				f.Type, f.Multi = Choice, true
			}
		default:
			panic(fmt.Sprintf("FieldsOf: %s.%s: unsupported kind %s", t, sf.Name, ft.Kind()))
		}
		if d, ok := sf.Tag.Lookup("default"); ok {
			if optional {
				panic(fmt.Sprintf("FieldsOf: %s.%s: optional fields cannot have a default", t, sf.Name))
			}
			v, err := coerce(f, d)
			if err != nil {
				panic(fmt.Sprintf("FieldsOf: %s.%s: bad default: %v", t, sf.Name, err))
			}
			f.Default = v
		} else {
			f.Required = !optional
		}
		add(f)
	}
	return out
}

// Decode fills out (a pointer to struct) from values. Unknown keys are errors
// unless listed in allow, so a typo in a declaration never passes silently.
func Decode(values map[string]any, out any, what string, allow ...string) error {
	rv := reflect.ValueOf(out).Elem()
	fields := FieldsOf(rv.Type())
	known := map[string]bool{}
	for _, a := range allow {
		known[a] = true
	}
	var problems []string
	for _, f := range fields {
		known[f.Key] = true
		raw, present := values[f.Key]
		if !present || raw == nil {
			if f.Default != nil {
				raw = f.Default
			} else if f.Required {
				problems = append(problems, fmt.Sprintf("missing %s %q", what, f.Key))
				continue
			} else {
				continue
			}
		}
		v, err := coerce(f, raw)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s %q: %v", what, f.Key, err))
			continue
		}
		set(rv.FieldByIndex(f.index), v)
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
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}

func set(dst reflect.Value, v any) {
	if dst.Kind() == reflect.Pointer {
		p := reflect.New(dst.Type().Elem())
		set(p.Elem(), v)
		dst.Set(p)
		return
	}
	switch dst.Kind() {
	case reflect.Int:
		dst.SetInt(int64(v.(float64)))
	default:
		dst.Set(reflect.ValueOf(v))
	}
}

// coerce turns a YAML/JSON/tag value into float64, string, bool or []string.
func coerce(f Field, raw any) (any, error) {
	switch f.Type {
	case Number:
		return toNumber(raw)
	case Flag:
		switch v := raw.(type) {
		case bool:
			return v, nil
		case string:
			return strconv.ParseBool(v)
		}
		return nil, fmt.Errorf("want a boolean, got %v", raw)
	case Text:
		return toText(raw)
	case List:
		return toList(raw)
	case Choice:
		if f.Multi {
			l, err := toList(raw)
			if err != nil {
				return nil, err
			}
			for _, s := range l {
				if err := oneOf(f.Options, s); err != nil {
					return nil, err
				}
			}
			return l, nil
		}
		s, err := toText(raw)
		if err != nil {
			return nil, err
		}
		return s, oneOf(f.Options, s)
	}
	return nil, fmt.Errorf("unknown field type %s", f.Type)
}

func toNumber(raw any) (float64, error) {
	var n float64
	switch v := raw.(type) {
	case float64:
		n = v
	case int:
		n = float64(v)
	case int64:
		n = float64(v)
	case uint64:
		n = float64(v)
	case string:
		p, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, fmt.Errorf("want a number, got %q", v)
		}
		n = p
	default:
		return 0, fmt.Errorf("want a number, got %v", raw)
	}
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, fmt.Errorf("want a finite number, got %v", n)
	}
	return n, nil
}

func toText(raw any) (string, error) {
	switch v := raw.(type) {
	case string:
		return v, nil
	case float64, int, bool:
		return fmt.Sprint(v), nil
	}
	return "", fmt.Errorf("want a string, got %v", raw)
}

func toList(raw any) ([]string, error) {
	switch v := raw.(type) {
	case []string:
		return v, nil
	case string:
		if v == "" {
			return []string{}, nil
		}
		return strings.Split(v, ","), nil
	case []any:
		out := make([]string, 0, len(v))
		for _, e := range v {
			s, err := toText(e)
			if err != nil {
				return nil, err
			}
			out = append(out, s)
		}
		return out, nil
	}
	return nil, fmt.Errorf("want a list of strings, got %v", raw)
}

func oneOf(options []string, s string) error {
	for _, o := range options {
		if o == s {
			return nil
		}
	}
	return fmt.Errorf("%q is not one of %s", s, strings.Join(options, ", "))
}
