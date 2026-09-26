package field

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
)

// coerce turns a YAML/JSON/tag value into float64, string, bool or []string.
func coerce(f Field, raw any) (any, error) {
	switch f.Type {
	case Number:
		return toNumber(raw)
	case Flag:
		return toFlag(raw)
	case Text:
		return toText(raw)
	case List:
		return toList(raw)
	case Choice:
		if f.Multi {
			return toChoices(f.Options, raw)
		}
		return toChoice(f.Options, raw)
	}
	return nil, fmt.Errorf("unknown field type %s", f.Type)
}

func toFlag(raw any) (any, error) {
	switch v := raw.(type) {
	case bool:
		return v, nil
	case string:
		return strconv.ParseBool(v)
	}
	return nil, fmt.Errorf("want a boolean, got %v", raw)
}

func toChoice(options []string, raw any) (any, error) {
	s, err := toText(raw)
	if err != nil {
		return nil, err
	}
	return s, oneOf(options, s)
}

func toChoices(options []string, raw any) (any, error) {
	l, err := toList(raw)
	if err != nil {
		return nil, err
	}
	for _, s := range l {
		if err := oneOf(options, s); err != nil {
			return nil, err
		}
	}
	return l, nil
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
	if slices.Contains(options, s) {
		return nil
	}
	return fmt.Errorf("%q is not one of %s", s, strings.Join(options, ", "))
}
