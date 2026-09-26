package field

import (
	"reflect"
	"strings"
	"testing"
)

type sample struct {
	Size   float64  `scout:"size" unit:"MB" default:"128"`
	Need   float64  `scout:"need"`
	Maybe  *float64 `scout:"maybe"`
	Mode   string   `scout:"mode" options:"a,b" default:"a"`
	Arch   []string `scout:"arch" options:"x86_64,arm64" default:"x86_64"`
	Strict bool     `scout:"strict" default:"false"`
}

func TestFieldsOf(t *testing.T) {
	fs := FieldsOf(reflect.TypeFor[sample]())
	byKey := map[string]Field{}
	for _, f := range fs {
		byKey[f.Key] = f
	}
	if !byKey["need"].Required || byKey["size"].Required || byKey["maybe"].Required {
		t.Fatalf("required flags wrong: %+v", fs)
	}
	if byKey["mode"].Type != Choice || byKey["arch"].Type != Choice || !byKey["arch"].Multi {
		t.Fatalf("choice detection wrong: %+v", fs)
	}
	if byKey["size"].Default != 128.0 {
		t.Fatalf("default not parsed: %v", byKey["size"].Default)
	}
}

func TestDecode(t *testing.T) {
	var s sample
	if err := Decode(map[string]any{"need": 3, "maybe": "2.5", "arch": []any{"arm64"}}, &s, "assumption"); err != nil {
		t.Fatal(err)
	}
	if s.Size != 128 || s.Need != 3 || s.Maybe == nil || *s.Maybe != 2.5 || s.Mode != "a" || s.Arch[0] != "arm64" {
		t.Fatalf("decoded %+v", s)
	}
}

func TestDecodeReportsEveryProblem(t *testing.T) {
	var s sample
	err := Decode(map[string]any{"need": nil, "mode": "c", "typo": 1}, &s, "assumption")
	if err == nil {
		t.Fatal("want an error")
	}
	for _, want := range []string{`missing assumption "need"`, `"c" is not one of a, b`, `unknown assumption "typo"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
}

type shared struct {
	Retention float64 `scout:"retention" default:"30"`
}

type composed struct {
	shared
	Own float64 `scout:"own"`
}

func TestEmbeddedStructsCompose(t *testing.T) {
	fs := FieldsOf(reflect.TypeFor[composed]())
	if len(fs) != 2 || fs[0].Key != "retention" || fs[1].Key != "own" {
		t.Fatalf("fields: %+v", fs)
	}
	var c composed
	if err := Decode(map[string]any{"own": 2}, &c, "assumption"); err != nil {
		t.Fatal(err)
	}
	if c.Retention != 30 || c.Own != 2 {
		t.Fatalf("decoded %+v", c)
	}
}

func TestSpecAndDecodeValues(t *testing.T) {
	var fields []Field
	for _, s := range []Spec{
		{Key: "size", Type: Number, Default: 128},
		{Key: "need", Type: Number},
		{Key: "maybe", Type: Number, Optional: true},
		{Key: "arch", Type: Choice, Options: []string{"x86_64", "arm64"}, Multi: true, Default: []any{"x86_64"}},
	} {
		f, err := s.Build()
		if err != nil {
			t.Fatal(err)
		}
		fields = append(fields, f)
	}
	got, err := DecodeValues(fields, map[string]any{"need": 2}, "assumption")
	if err != nil {
		t.Fatal(err)
	}
	if got["size"] != 128.0 || got["need"] != 2.0 || got["arch"].([]string)[0] != "x86_64" {
		t.Fatalf("got %v", got)
	}
	if _, set := got["maybe"]; set {
		t.Fatal("an unset optional field must be absent")
	}
	if _, err := DecodeValues(fields, map[string]any{"typo": 1}, "assumption"); err == nil {
		t.Fatal("want missing and unknown keys to fail")
	}
	if _, err := (Spec{Key: "x", Type: Choice}).Build(); err == nil {
		t.Fatal("a choice without options must fail")
	}
}
func TestLabelsNameOptions(t *testing.T) {
	s := Spec{Key: "plan", Type: Choice, Options: []string{"a", "b"}, Labels: map[string]string{"a": "4 GB, 4 cores"}}
	f, err := s.Build()
	if err != nil || f.Labels["a"] != "4 GB, 4 cores" {
		t.Fatalf("labels = %v, %v", f.Labels, err)
	}
	s.Labels["c"] = "not an option"
	if _, err := s.Build(); err == nil {
		t.Error("a label for an option the choice lacks should be refused")
	}
}
