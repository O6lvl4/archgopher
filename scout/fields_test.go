package scout

import (
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
	fs := FieldsOf(typeOf[sample]())
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
