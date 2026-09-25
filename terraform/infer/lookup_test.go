package infer

import (
	"github.com/O6lvl4/archgopher/terraform/eval"
	"testing"

	"github.com/O6lvl4/archgopher/field"
)

func TestLookupPathSelectsABlockByKey(t *testing.T) {
	attrs := map[string]any{
		"setting": []any{
			map[string]any{"namespace": "aws:autoscaling:asg", "name": "MinSize", "value": "2"},
			map[string]any{"namespace": "aws:autoscaling:launchconfiguration", "name": "InstanceType", "value": "t3.small"},
		},
		"root": []any{map[string]any{"size": 8.0}},
	}
	for _, c := range []struct {
		path string
		want any
	}{
		{"setting[name=InstanceType].value", "t3.small"},
		{"setting[name=MinSize].value", "2"},
		{"setting[name=MaxSize].value", nil},
		{"setting.value", "2"},
		{"root.size", 8.0},
		{"root[size=8].size", 8.0},
		{"missing[name=x].value", nil},
	} {
		if got := lookupPath(attrs, c.path); got != c.want {
			t.Errorf("%s: got %v, want %v", c.path, got, c.want)
		}
	}
}

// A boolean that points at a value that is not one (an id written as a
// literal) reads whether the value is written.
func TestFlagOfAWrittenValue(t *testing.T) {
	read := func(f field.Field, v any) any {
		return (&builder{}).readAttribute(&eval.Resource{Attrs: map[string]any{f.Key: v}}, f)
	}
	flag := field.Field{Key: "on_host", Type: field.Flag}
	for _, c := range []struct {
		in, want any
	}{
		{"h-0123", true},
		{"", nil},
		{"true", true},
		{"false", false},
		{true, true},
	} {
		if got := read(flag, c.in); got != c.want {
			t.Errorf("%v: got %v, want %v", c.in, got, c.want)
		}
	}
	if got := read(field.Field{Key: "name", Type: field.Text}, "h-0123"); got != "h-0123" {
		t.Errorf("text fields keep their value, got %v", got)
	}
}
