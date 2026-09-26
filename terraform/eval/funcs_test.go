package eval

import (
	"testing"

	"github.com/zclconf/go-cty/cty"
)

func TestLookupTakesANullDefault(t *testing.T) {
	m := cty.MapVal(map[string]cty.Value{"g2l-t-c6m12": cty.StringVal("uuid")})
	cases := []struct {
		key  string
		def  cty.Value
		want cty.Value
	}{
		{"g2l-t-c6m12", cty.NullVal(cty.String), cty.StringVal("uuid")},
		{"g2l-t-c8m24", cty.NullVal(cty.String), cty.NullVal(cty.String)},
		{"g2l-t-c8m24", cty.StringVal("published"), cty.StringVal("published")},
	}
	for _, c := range cases {
		got, err := lookupFunc.Call([]cty.Value{m, cty.StringVal(c.key), c.def})
		if err != nil || !got.RawEquals(c.want) {
			t.Errorf("lookup(%s, %#v) = %#v, %v; want %#v", c.key, c.def, got, err, c.want)
		}
	}
	if got, err := lookupFunc.Call([]cty.Value{m, cty.StringVal("g2l-t-c6m12")}); err != nil || got.AsString() != "uuid" {
		t.Errorf("lookup without a default = %#v, %v", got, err)
	}
	if _, err := lookupFunc.Call([]cty.Value{m, cty.StringVal("missing")}); err == nil {
		t.Error("a missing key without a default should fail")
	}
	if got, _ := lookupFunc.Call([]cty.Value{cty.DynamicVal, cty.StringVal("k"), cty.NullVal(cty.String)}); got.IsKnown() {
		t.Errorf("an unknown map should look up unknown, got %#v", got)
	}
}
