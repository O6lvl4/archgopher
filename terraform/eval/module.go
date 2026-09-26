package eval

import (
	"fmt"
	"strings"

	"github.com/O6lvl4/archgopher/terraform/config"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
)

// maxPasses bounds the rounds of locals and module calls: each round can
// only make more values known, and real configurations settle in two or three.
const maxPasses = 6

// evalInstance evaluates the locals, module calls and outputs of an
// instance. Locals and calls can depend on each other in any order, so they
// are evaluated again until nothing changes.
func (ev *evaluator) evalInstance(in *instance) {
	in.locals = map[string]cty.Value{}
	in.children = map[string]*instance{}
	in.counted = map[string]bool{}
	in.removed = map[string]bool{}
	for name := range in.mod.Locals {
		in.locals[name] = cty.DynamicVal
	}
	for pass := 0; pass < maxPasses; pass++ {
		localsChanged := ev.settleLocals(in)
		callsChanged := ev.evalCalls(in)
		if !localsChanged && !callsChanged && pass > 0 {
			break
		}
	}
	in.outputs = map[string]cty.Value{}
	for name, expr := range in.mod.Outputs {
		in.outputs[name] = ev.eval(expr, in, nil)
	}
}

// settleLocals evaluates the locals until they stop changing, at most once
// more than there are locals, and reports whether any changed.
func (ev *evaluator) settleLocals(in *instance) bool {
	changed := false
	for i := 0; i <= len(in.mod.Locals); i++ {
		if !ev.evalLocals(in) {
			break
		}
		changed = true
	}
	return changed
}

// evalLocals evaluates every local once and reports whether any changed.
func (ev *evaluator) evalLocals(in *instance) bool {
	changed := false
	for name, expr := range in.mod.Locals {
		v := ev.eval(expr, in, nil)
		if !v.RawEquals(in.locals[name]) {
			in.locals[name], changed = v, true
		}
	}
	return changed
}

// evalCalls evaluates every module call and reports whether any outputs changed.
func (ev *evaluator) evalCalls(in *instance) bool {
	changed := false
	for _, call := range in.mod.Calls {
		if ev.evalCall(in, call) {
			changed = true
		}
	}
	return changed
}

// evalCall evaluates one module call and reports whether its outputs changed.
func (ev *evaluator) evalCall(in *instance, call *config.ModuleCall) bool {
	n := ev.instances(call.Count, call.ForEach, in)
	if n == 0 {
		changed := !in.removed[call.Name]
		in.removed[call.Name] = true
		delete(in.children, call.Name)
		return changed
	}
	in.counted[call.Name] = call.Count != nil || call.ForEach != nil
	key, mod, err := ev.loadCall(in, call)
	if err != nil {
		ev.warn("%v", err)
		return false
	}
	args := callArgs(call)
	child := &instance{mod: mod, path: key, parent: in, callArgs: args, vars: ev.childVars(mod, args, in), copies: times(in.copies, n)}
	ev.evalInstance(child)
	prev, had := in.children[call.Name]
	in.children[call.Name] = child
	if !had {
		return true
	}
	return !cty.ObjectVal(orEmpty(prev.outputs)).RawEquals(cty.ObjectVal(orEmpty(child.outputs)))
}

// loadCall resolves and parses the module a call points at.
func (ev *evaluator) loadCall(in *instance, call *config.ModuleCall) ([]string, *config.Module, error) {
	key := in.callKey(call)
	dir, err := ev.ld.Resolve(in.mod.Dir, call, key)
	if err != nil {
		return nil, nil, err
	}
	mod, err := ev.ld.Load(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("module %s: %w", strings.Join(key, "."), err)
	}
	return key, mod, nil
}

// callMeta are the arguments of a module block that Terraform reads itself
// rather than passing to the module as variables.
var callMeta = map[string]bool{
	"source": true, "version": true, "count": true, "for_each": true, "providers": true, "depends_on": true,
}

// callArgs returns the variables a module call passes.
func callArgs(call *config.ModuleCall) map[string]hcl.Expression {
	args := map[string]hcl.Expression{}
	for name, a := range call.Body.Attributes {
		if !callMeta[name] {
			args[name] = a.Expr
		}
	}
	return args
}

// childVars gives each variable of the called module the value passed in,
// else its default, else unknown.
func (ev *evaluator) childVars(mod *config.Module, args map[string]hcl.Expression, in *instance) map[string]cty.Value {
	vars := map[string]cty.Value{}
	for name, def := range mod.Variables {
		switch expr, ok := args[name]; {
		case ok:
			vars[name] = ev.eval(expr, in, nil)
		case def != nil:
			vars[name] = ev.eval(def, nil, nil)
		default:
			vars[name] = cty.DynamicVal
		}
	}
	return vars
}

// instances evaluates count / for_each: UnknownInstances, otherwise the size.
func (ev *evaluator) instances(count, forEach hcl.Expression, in *instance) int {
	if count != nil {
		v := ev.eval(count, in, nil)
		if !v.IsWhollyKnown() || v.IsNull() || !v.Type().Equals(cty.Number) {
			return UnknownInstances
		}
		f, _ := v.AsBigFloat().Int64()
		return int(f)
	}
	if forEach != nil {
		v := ev.eval(forEach, in, nil)
		if !v.IsKnown() || v.IsNull() || !v.CanIterateElements() {
			return UnknownInstances
		}
		return v.LengthInt()
	}
	return 1
}
