package wdl

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCount(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		want    Actions
		wantErr string
	}{
		{name: "a built-in action", src: `{"type":"Http"}`, want: Actions{BuiltIn: 1}},
		{name: "a connector call", src: `{"type":"ApiConnection"}`, want: Actions{Connector: 1}},
		{
			name: "a scope runs all its actions",
			src:  `{"type":"Scope","actions":{"a":{"type":"Compose"},"b":{"type":"ApiConnection"}}}`,
			want: Actions{BuiltIn: 2, Connector: 1},
		},
		{
			name: "a condition takes the branch with more connector calls",
			src:  `{"type":"If","actions":{"a":{"type":"Compose"},"b":{"type":"Compose"},"c":{"type":"Compose"}},"else":{"actions":{"d":{"type":"ApiConnection"}}}}`,
			want: Actions{BuiltIn: 1, Connector: 1},
		},
		{
			name: "a switch takes its longest case, the default included",
			src:  `{"type":"Switch","cases":{"x":{"actions":{"a":{"type":"Compose"}}}},"default":{"actions":{"b":{"type":"Compose"},"c":{"type":"Http"}}}}`,
			want: Actions{BuiltIn: 3},
		},
		{name: "a loop", src: `{"type":"Foreach","actions":{"a":{"type":"Compose"}}}`, wantErr: `action "x" is a Foreach loop`},
		{name: "a loop inside a condition", src: `{"type":"If","actions":{"l":{"type":"Until"}}}`, wantErr: `action "l" is a Until loop`},
		{name: "a type not known yet", src: `{"type":"Teleport"}`, wantErr: `action "x" has type "Teleport", which is not counted yet`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var doc any
			if err := json.Unmarshal([]byte(c.src), &doc); err != nil {
				t.Fatal(err)
			}
			got, err := Count("x", doc)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("err = %v, want %q", err, c.wantErr)
				}
				return
			}
			if err != nil || got != c.want {
				t.Fatalf("Count = %+v, %v; want %+v", got, err, c.want)
			}
		})
	}
}
