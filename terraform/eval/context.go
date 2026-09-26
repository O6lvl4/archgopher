package eval

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
)

// ctx builds the evaluation context of a module instance. Without an
// instance (variable defaults) only the names Terraform always defines exist.
func (ev *evaluator) ctx(in *instance, extra map[string]cty.Value) *hcl.EvalContext {
	vars := map[string]cty.Value{
		"count":     cty.ObjectVal(map[string]cty.Value{"index": cty.NumberIntVal(0)}),
		"each":      cty.ObjectVal(map[string]cty.Value{"key": cty.DynamicVal, "value": cty.DynamicVal}),
		"self":      cty.DynamicVal,
		"terraform": cty.ObjectVal(map[string]cty.Value{"workspace": cty.StringVal("default")}),
	}
	if in != nil {
		ev.addInstanceVars(vars, in)
	}
	for k, v := range extra {
		vars[k] = v
	}
	return &hcl.EvalContext{Variables: vars, Functions: ev.funcs}
}

// addInstanceVars adds what a module instance defines: var, local, path,
// module and its resources and data sources, which stay unknown.
func (ev *evaluator) addInstanceVars(vars map[string]cty.Value, in *instance) {
	vars["var"] = cty.ObjectVal(orEmpty(in.vars))
	vars["local"] = cty.ObjectVal(orEmpty(in.locals))
	vars["path"] = cty.ObjectVal(map[string]cty.Value{
		"module": cty.StringVal(in.mod.Dir), "root": cty.StringVal(ev.rootDir), "cwd": cty.StringVal(ev.rootDir),
	})
	vars["module"] = moduleObject(in)
	managed, data := resourceObjects(in)
	for t, names := range managed {
		vars[t] = names
	}
	vars["data"] = cty.ObjectVal(data)
}

// moduleObject is the value of module.<call>: an empty tuple when the call
// is off, unknown while it is counted or not evaluated, its outputs otherwise.
func moduleObject(in *instance) cty.Value {
	mods := map[string]cty.Value{}
	for _, call := range in.mod.Calls {
		child, ok := in.children[call.Name]
		switch {
		case in.removed[call.Name]:
			mods[call.Name] = cty.EmptyTupleVal
		case !ok || in.counted[call.Name]:
			mods[call.Name] = cty.DynamicVal
		default:
			mods[call.Name] = cty.ObjectVal(orEmpty(child.outputs))
		}
	}
	return cty.ObjectVal(mods)
}

// resourceObjects returns, by type, an object of unknown values for every
// managed resource and every data source the instance declares.
func resourceObjects(in *instance) (managed, data map[string]cty.Value) {
	managedNames := map[string]map[string]cty.Value{}
	dataNames := map[string]map[string]cty.Value{}
	for _, r := range in.mod.Resources {
		target := managedNames
		if r.Mode == "data" {
			target = dataNames
		}
		if target[r.Type] == nil {
			target[r.Type] = map[string]cty.Value{}
		}
		target[r.Type][r.Name] = cty.DynamicVal
	}
	return objectsByType(managedNames), objectsByType(dataNames)
}

func objectsByType(m map[string]map[string]cty.Value) map[string]cty.Value {
	out := map[string]cty.Value{}
	for t, names := range m {
		out[t] = cty.ObjectVal(names)
	}
	return out
}

func orEmpty(m map[string]cty.Value) map[string]cty.Value {
	if m == nil {
		return map[string]cty.Value{}
	}
	return m
}
