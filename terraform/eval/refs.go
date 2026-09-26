package eval

import (
	"github.com/hashicorp/hcl/v2"
)

// refs returns the resource addresses an expression references, following
// variables, locals and module outputs to where they are defined.
func (ev *evaluator) refs(expr hcl.Expression, in *instance) []string {
	set := map[string]bool{}
	for _, t := range expr.Variables() {
		for _, a := range ev.resolve(t, in) {
			set[a] = true
		}
	}
	return sortedKeys(set)
}

// resolve returns the resources one traversal references.
func (ev *evaluator) resolve(t hcl.Traversal, in *instance) []string {
	root := t.RootName()
	name := step(t, 1)
	switch root {
	case "count", "each", "path", "terraform", "self":
		return nil
	case "var":
		return ev.resolveVar(in, name)
	case "local":
		expr, ok := in.mod.Locals[name]
		if !ok {
			return nil
		}
		return ev.memoRefs(in.addr()+"local."+name, expr, in)
	case "module":
		return ev.resolveModule(t, in, name)
	case "data":
		return in.resourceRef("data", name, step(t, 2))
	}
	return in.resourceRef("managed", root, name)
}

// resolveVar follows a variable to the argument the parent passed; root
// variables reference nothing.
func (ev *evaluator) resolveVar(in *instance, name string) []string {
	if in.parent == nil || name == "" {
		return nil
	}
	expr, ok := in.callArgs[name]
	if !ok {
		return nil
	}
	return ev.memoRefs(in.addr()+"var."+name, expr, in.parent)
}

// resolveModule follows module.<name>.<output> into the child; without a
// readable output name it takes every output of the child.
func (ev *evaluator) resolveModule(t hcl.Traversal, in *instance, name string) []string {
	child, ok := in.children[name]
	if !ok {
		return nil
	}
	// module.x[0].out and module.x["k"].out skip the index step
	out := step(t, 2)
	if out == "" {
		out = step(t, 3)
	}
	if expr, ok := child.mod.Outputs[out]; ok {
		return ev.memoRefs(child.addr()+"output."+out, expr, child)
	}
	var all []string
	for o, expr := range child.mod.Outputs {
		all = append(all, ev.memoRefs(child.addr()+"output."+o, expr, child)...)
	}
	return all
}

// resourceRef is the address of a block the instance declares, or nothing.
func (in *instance) resourceRef(mode, typ, name string) []string {
	if !in.mod.Has(mode, typ, name) {
		return nil
	}
	return []string{address(in.addr(), mode, typ, name)}
}

// memoRefs resolves a named definition once; a definition that reaches
// itself contributes nothing the second time round.
func (ev *evaluator) memoRefs(key string, expr hcl.Expression, in *instance) []string {
	if r, ok := ev.memo[key]; ok {
		return r
	}
	if ev.visiting[key] {
		return nil
	}
	ev.visiting[key] = true
	r := ev.refs(expr, in)
	delete(ev.visiting, key)
	ev.memo[key] = r
	return r
}

func step(t hcl.Traversal, i int) string {
	if i >= len(t) {
		return ""
	}
	if s, ok := t[i].(hcl.TraverseAttr); ok {
		return s.Name
	}
	return "" // an index step: module.x[0].out
}
