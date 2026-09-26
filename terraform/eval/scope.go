package eval

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// Scope is where a resource block was evaluated: its module instance and the
// count / each values in effect. Provider packages use it to read parts of a
// block that the generic evaluator has no opinion on, such as IAM policies.
type Scope struct {
	ev    *evaluator
	in    *instance
	extra map[string]cty.Value
}

// Eval evaluates an expression; errors become unknown.
func (s *Scope) Eval(expr hcl.Expression) cty.Value { return s.ev.eval(expr, s.in, s.extra) }

// Refs returns the resource addresses an expression references, through
// variables, locals and module outputs.
func (s *Scope) Refs(expr hcl.Expression) []string { return s.ev.refs(expr, s.in) }

// With returns a scope with one more variable, such as a dynamic block iterator.
func (s *Scope) With(name string, v cty.Value) *Scope {
	extra := map[string]cty.Value{}
	for k, x := range s.extra {
		extra[k] = x
	}
	extra[name] = v
	return &Scope{ev: s.ev, in: s.in, extra: extra}
}

// Element is one element of a for_each collection and where it sits.
type Element struct {
	Key, Value cty.Value
	Step       PathStep
}

// Elements evaluates a for_each expression. ok is false when the collection is unknown.
func (s *Scope) Elements(forEach hcl.Expression) (elems []Element, ok bool) {
	v := s.Eval(forEach)
	if !v.IsKnown() || v.IsNull() || !v.CanIterateElements() {
		return nil, false
	}
	keyed := v.Type().IsMapType() || v.Type().IsObjectType()
	i := 0
	for it := v.ElementIterator(); it.Next(); i++ {
		k, e := it.Element()
		step := PathStep{Index: i}
		if keyed {
			step = PathStep{Attr: k.AsString(), Index: -1}
		}
		elems = append(elems, Element{Key: k, Value: e, Step: step})
	}
	return elems, true
}

// IterRefs resolves references in expr where iter is a dynamic block iterator
// over forEach, positioned at elem (use Every for all elements).
func (s *Scope) IterRefs(expr hcl.Expression, iter string, forEach hcl.Expression, elem PathStep) []string {
	return s.ev.iterRefs(expr, s.in, iterator{name: iter, forEach: forEach, elem: elem})
}

// Every is the element step meaning "any element": references of the whole collection.
var Every = PathStep{Index: -2}

// DynamicContent returns the content body of a dynamic block.
func DynamicContent(blk *hclsyntax.Block) *hclsyntax.Body { return dynamicContent(blk) }

// IteratorName returns the iterator variable of a dynamic block.
func IteratorName(blk *hclsyntax.Block) string { return iteratorName(blk) }

// KeyName reads an object key written bare or quoted.
func KeyName(expr hclsyntax.Expression) string { return keyName(expr) }

// ToGo converts the known parts of a value.
func ToGo(v cty.Value) any { return toGo(v) }

// Strings reads a string or a list of strings; unknown parts are dropped.
func Strings(v cty.Value) []string {
	switch x := toGo(v).(type) {
	case string:
		return []string{x}
	case []any:
		out := []string{}
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// String reads a known string, or "".
func String(v cty.Value) string {
	if !v.IsKnown() || v.IsNull() || v.Type() != cty.String {
		return ""
	}
	return v.AsString()
}

func keyName(expr hclsyntax.Expression) string {
	if kw := hcl.ExprAsKeyword(expr); kw != "" {
		return kw
	}
	if v, diags := expr.Value(nil); !diags.HasErrors() && v.Type() == cty.String && v.IsKnown() {
		return v.AsString()
	}
	return ""
}

func iteratorName(blk *hclsyntax.Block) string {
	a, ok := blk.Body.Attributes["iterator"]
	if !ok {
		return blk.Labels[0]
	}
	if t, diags := hcl.AbsTraversalForExpr(a.Expr); !diags.HasErrors() {
		return t.RootName()
	}
	return blk.Labels[0]
}

// IteratorValue is the object a dynamic block iterator holds for one element.
func IteratorValue(el Element) cty.Value {
	return cty.ObjectVal(map[string]cty.Value{"key": el.Key, "value": el.Value})
}
