package field

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// Decode fills out (a pointer to struct) from values. Unknown keys are errors
// unless listed in allow, so a typo in a declaration never passes silently.
func Decode(values map[string]any, out any, what string, allow ...string) error {
	rv := reflect.ValueOf(out).Elem()
	fields := FieldsOf(rv.Type())
	// Values that decoded are set even when others did not.
	decoded, err := decode(fields, values, what, allow)
	for _, f := range fields {
		if v, ok := decoded[f.Key]; ok {
			set(rv.FieldByIndex(f.index), v)
		}
	}
	return err
}

// DecodeValues validates values against fields and returns them normalized:
// numbers as float64, text as string, flags as bool, lists as []string.
// Defaults are applied; optional fields that are not set are absent. Unknown
// keys are errors unless listed in allow.
func DecodeValues(fields []Field, values map[string]any, what string, allow ...string) (map[string]any, error) {
	out, err := decode(fields, values, what, allow)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// decode coerces each field's value, or its default, and returns those that
// coerced by key. It reports every missing, malformed and unknown value at once.
func decode(fields []Field, values map[string]any, what string, allow []string) (map[string]any, error) {
	out := map[string]any{}
	known := map[string]bool{}
	for _, a := range allow {
		known[a] = true
	}
	var problems []string
	for _, f := range fields {
		known[f.Key] = true
		raw, ok := rawValue(f, values)
		if !ok {
			if f.Required {
				problems = append(problems, fmt.Sprintf("missing %s %q", what, f.Key))
			}
			continue
		}
		v, err := coerce(f, raw)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s %q: %v", what, f.Key, err))
			continue
		}
		out[f.Key] = v
	}
	problems = append(problems, unknownKeys(values, known, what)...)
	if len(problems) > 0 {
		return out, fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return out, nil
}

// rawValue is the value given for f, or its default; false if there is neither.
func rawValue(f Field, values map[string]any) (any, bool) {
	if raw := values[f.Key]; raw != nil {
		return raw, true
	}
	if f.Default != nil {
		return f.Default, true
	}
	return nil, false
}

// unknownKeys reports, in order, the keys of values that are not known.
func unknownKeys(values map[string]any, known map[string]bool, what string) []string {
	var unknown []string
	for k := range values {
		if !known[k] {
			unknown = append(unknown, k)
		}
	}
	sort.Strings(unknown)
	problems := make([]string, 0, len(unknown))
	for _, k := range unknown {
		problems = append(problems, fmt.Sprintf("unknown %s %q", what, k))
	}
	return problems
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
