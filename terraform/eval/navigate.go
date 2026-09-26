package eval

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// PathStep is one step into a structure: an attribute/map key or a list index.
type PathStep struct {
	Attr  string
	Index int // -1 for attribute steps; -2 for "every element"
}

// navigate finds the references of one element inside a structure, following
// the structure through variables, locals and module outputs. It lets
// `dynamic "statement" { for_each = var.statements }` attribute each
// statement's resources to that statement only. When the structure cannot be
// followed it falls back to every reference of the expression.
func (ev *evaluator) navigate(expr hcl.Expression, in *instance, path []PathStep) []string {
	if len(path) == 0 {
		return ev.refs(expr, in)
	}
	switch e := expr.(type) {
	case *hclsyntax.TemplateWrapExpr:
		return ev.navigate(e.Wrapped, in, path)
	case *hclsyntax.TupleConsExpr:
		if i := path[0].Index; i >= 0 && i < len(e.Exprs) {
			return ev.navigate(e.Exprs[i], in, path[1:])
		}
	case *hclsyntax.ObjectConsExpr:
		if path[0].Index == -1 {
			for _, item := range e.Items {
				if keyName(item.KeyExpr) == path[0].Attr {
					return ev.navigate(item.ValueExpr, in, path[1:])
				}
			}
			return nil
		}
	case *hclsyntax.ScopeTraversalExpr:
		if target, tin, rest, ok := ev.definition(e.Traversal, in); ok {
			return ev.navigate(target, tin, append(rest, path...))
		}
	}
	return ev.refs(expr, in)
}

// definition resolves var.x / local.x / module.m.out to the expression that defines it.
func (ev *evaluator) definition(t hcl.Traversal, in *instance) (hcl.Expression, *instance, []PathStep, bool) {
	name := step(t, 1)
	switch t.RootName() {
	case "var":
		if in.parent == nil {
			return nil, nil, nil, false
		}
		if expr, ok := in.callArgs[name]; ok {
			return expr, in.parent, steps(t[2:]), true
		}
	case "local":
		if expr, ok := in.mod.Locals[name]; ok {
			return expr, in, steps(t[2:]), true
		}
	case "module":
		child, ok := in.children[name]
		if !ok || len(t) < 3 {
			break
		}
		out := step(t, 2)
		if expr, ok := child.mod.Outputs[out]; ok {
			return expr, child, steps(t[3:]), true
		}
	}
	return nil, nil, nil, false
}

func steps(t hcl.Traversal) []PathStep {
	var out []PathStep
	for _, s := range t {
		switch s := s.(type) {
		case hcl.TraverseAttr:
			out = append(out, PathStep{Attr: s.Name, Index: -1})
		case hcl.TraverseIndex:
			k := s.Key
			switch {
			case !k.IsKnown() || k.IsNull():
				return out
			case k.Type() == cty.Number:
				i, _ := k.AsBigFloat().Int64()
				out = append(out, PathStep{Index: int(i)})
			case k.Type() == cty.String:
				out = append(out, PathStep{Attr: k.AsString(), Index: -1})
			default:
				return out
			}
		default:
			return out
		}
	}
	return out
}

// iterator is a dynamic block's iterator positioned at one element of the
// collection it ranges over.
type iterator struct {
	name    string         // the iterator variable, the block label unless renamed
	forEach hcl.Expression // the collection
	elem    PathStep       // the current element; Every for all of them
}

// iterRefs resolves references in expr where the root it.name is the iterator
// of a dynamic block whose current element is at it.elem inside it.forEach.
func (ev *evaluator) iterRefs(expr hcl.Expression, in *instance, it iterator) []string {
	set := map[string]bool{}
	for _, t := range expr.Variables() {
		var refs []string
		switch {
		case t.RootName() != it.name:
			refs = ev.resolve(t, in)
		case step(t, 1) == "value":
			refs = ev.navigate(it.forEach, in, append([]PathStep{it.elem}, steps(t[2:])...))
		}
		for _, r := range refs {
			set[r] = true
		}
	}
	return sortedKeys(set)
}
