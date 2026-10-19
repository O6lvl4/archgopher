package eval

import (
	"github.com/hashicorp/hcl/v2"
)

// A resource or module call whose count or for_each is 0 is left out, and the
// warning that lists it should say which root variables decide that: the
// user passes --var to bring it in, and the name is what they need.

// rootVars lists the root module's variables an expression depends on, as
// "var.<name>", following module arguments and locals back to the root.
// Variables of a module that the call does not set contribute nothing.
func (ev *evaluator) rootVars(expr hcl.Expression, in *instance) []string {
	set := map[string]bool{}
	ev.collectRootVars(expr, in, set, map[string]bool{})
	return sortedKeys(set)
}

func (ev *evaluator) collectRootVars(expr hcl.Expression, in *instance, set, visiting map[string]bool) {
	for _, t := range expr.Variables() {
		name := step(t, 1)
		switch t.RootName() {
		case "var":
			if in.parent == nil {
				if name != "" {
					set["var."+name] = true
				}
			} else if arg, ok := in.callArgs[name]; ok {
				ev.collectRootVars(arg, in.parent, set, visiting)
			}
		case "local":
			key := in.addr() + "local." + name
			if local, ok := in.mod.Locals[name]; ok && !visiting[key] {
				visiting[key] = true
				ev.collectRootVars(local, in, set, visiting)
				delete(visiting, key)
			}
		case "module":
			child, ok := in.children[name]
			if !ok {
				continue
			}
			out := step(t, 2)
			if out == "" {
				out = step(t, 3)
			}
			if o, ok := child.mod.Outputs[out]; ok {
				ev.collectRootVars(o, child, set, visiting)
			}
		}
	}
}

// offBy is the variables that switch a count or for_each off.
func (ev *evaluator) offBy(count, forEach hcl.Expression, in *instance) []string {
	if count != nil {
		return ev.rootVars(count, in)
	}
	if forEach != nil {
		return ev.rootVars(forEach, in)
	}
	return nil
}
