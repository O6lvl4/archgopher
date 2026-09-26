package eval

import (
	"math/big"
	"sort"

	"github.com/zclconf/go-cty/cty"
)

// toGo converts the known parts of a value; unknown and null become nil.
func toGo(v cty.Value) any {
	if v.IsMarked() {
		v, _ = v.Unmark()
	}
	if !v.IsKnown() || v.IsNull() {
		return nil
	}
	t := v.Type()
	switch {
	case t == cty.String:
		return v.AsString()
	case t == cty.Number:
		return numberToGo(v.AsBigFloat())
	case t == cty.Bool:
		return v.True()
	case t.IsListType() || t.IsTupleType() || t.IsSetType():
		return listToGo(v)
	case t.IsMapType() || t.IsObjectType():
		return mapToGo(v)
	}
	return nil
}

// numberToGo is an int when the number is one that fits, a float64 otherwise.
func numberToGo(n *big.Float) any {
	if i, acc := n.Int64(); n.IsInt() && acc == big.Exact {
		return int(i)
	}
	f, _ := n.Float64()
	return f
}

// listToGo converts the known elements of a list, tuple or set.
func listToGo(v cty.Value) []any {
	out := []any{}
	for it := v.ElementIterator(); it.Next(); {
		_, e := it.Element()
		if g := toGo(e); g != nil {
			out = append(out, g)
		}
	}
	return out
}

// mapToGo converts the known entries of a map or object.
func mapToGo(v cty.Value) map[string]any {
	out := map[string]any{}
	for it := v.ElementIterator(); it.Next(); {
		k, e := it.Element()
		if g := toGo(e); g != nil {
			out[k.AsString()] = g
		}
	}
	return out
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func union(a, b []string) []string {
	set := map[string]bool{}
	for _, x := range a {
		set[x] = true
	}
	for _, x := range b {
		set[x] = true
	}
	return sortedKeys(set)
}
