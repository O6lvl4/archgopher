package terraform

import (
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

// Resource is one evaluated resource block (instance 0 when counted).
type Resource struct {
	Address string
	Mode    string
	Type    string
	Name    string
	Module  string
	// Attrs holds the values known before apply. Nested blocks are lists of maps.
	Attrs map[string]any
	// Refs maps an attribute path ("environment.variables") to the resources it references.
	Refs map[string][]string
	// Statements are IAM policy statements found in the resource.
	Statements []Statement
	// Instances is the count or for_each size; -1 when unknown.
	Instances int
}

// Statement is one Allow statement of an IAM policy.
type Statement struct {
	// Actions is nil when the actions could not be read.
	Actions []string
	Targets []string
	// Docs are policy documents (data sources) the statement defers to.
	Docs []string
}

// Options tunes evaluation.
type Options struct {
	VarFiles []string
	Vars     map[string]string
}

// Evaluated is everything read from a root module.
type Evaluated struct {
	Resources []*Resource
	// Removed are resources whose count or for_each is 0, including every
	// resource of a module call that is off.
	Removed  []*Resource
	Region   string
	Warnings []string
}

// Evaluate reads the root module in dir and every module it calls.
func Evaluate(dir string, opt Options) (*Evaluated, error) {
	ld := newLoader(dir)
	root, err := ld.load(dir)
	if err != nil {
		return nil, err
	}
	ev := &evaluator{ld: ld, rootDir: dir, funcs: functions(), memo: map[string][]string{}, visiting: map[string]bool{}}
	vars, err := rootVars(root, dir, opt)
	if err != nil {
		return nil, err
	}
	in := &instance{mod: root, vars: vars}
	ev.evalInstance(in)
	out := &Evaluated{}
	ev.collect(in, out)
	out.Region = ev.region(in)
	out.Warnings = ev.warnings
	sort.Slice(out.Resources, func(i, j int) bool { return out.Resources[i].Address < out.Resources[j].Address })
	return out, nil
}

type instance struct {
	mod      *Module
	path     []string // module call names from the root
	parent   *instance
	callArgs map[string]hcl.Expression
	vars     map[string]cty.Value
	locals   map[string]cty.Value
	children map[string]*instance
	outputs  map[string]cty.Value
	// counted children have count or for_each; their outputs are left unknown
	counted map[string]bool
	removed map[string]bool
}

func (in *instance) addr() string {
	var b strings.Builder
	for _, p := range in.path {
		b.WriteString("module." + p + ".")
	}
	return b.String()
}

type evaluator struct {
	ld       *loader
	rootDir  string
	funcs    map[string]function.Function
	warnings []string
	warned   map[string]bool
	memo     map[string][]string
	visiting map[string]bool
}

func (ev *evaluator) warn(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if ev.warned == nil {
		ev.warned = map[string]bool{}
	}
	if !ev.warned[msg] {
		ev.warned[msg] = true
		ev.warnings = append(ev.warnings, msg)
	}
}

const maxPasses = 6

func (ev *evaluator) evalInstance(in *instance) {
	in.locals = map[string]cty.Value{}
	in.children = map[string]*instance{}
	in.counted = map[string]bool{}
	in.removed = map[string]bool{}
	for name := range in.mod.Locals {
		in.locals[name] = cty.DynamicVal
	}
	for pass := 0; pass < maxPasses; pass++ {
		changed := false
		for i := 0; i <= len(in.mod.Locals); i++ {
			step := false
			for name, expr := range in.mod.Locals {
				v := ev.eval(expr, in, nil)
				if !v.RawEquals(in.locals[name]) {
					in.locals[name], step = v, true
				}
			}
			if !step {
				break
			}
			changed = true
		}
		for _, call := range in.mod.Calls {
			if ev.evalCall(in, call) {
				changed = true
			}
		}
		if !changed && pass > 0 {
			break
		}
	}
	in.outputs = map[string]cty.Value{}
	for name, expr := range in.mod.Outputs {
		in.outputs[name] = ev.eval(expr, in, nil)
	}
}

// evalCall evaluates one module call and reports whether its outputs changed.
func (ev *evaluator) evalCall(in *instance, call *ModuleCall) bool {
	n := ev.instances(call.Count, call.ForEach, in)
	if n == 0 {
		changed := !in.removed[call.Name]
		in.removed[call.Name] = true
		delete(in.children, call.Name)
		return changed
	}
	in.counted[call.Name] = call.Count != nil || call.ForEach != nil
	key := append(append([]string(nil), in.path...), call.Name)
	dir, err := ev.ld.resolve(in.mod.Dir, call, key)
	if err != nil {
		ev.warn("%v", err)
		return false
	}
	mod, err := ev.ld.load(dir)
	if err != nil {
		ev.warn("module %s: %v", strings.Join(key, "."), err)
		return false
	}
	args := map[string]hcl.Expression{}
	vars := map[string]cty.Value{}
	for name, a := range call.Body.Attributes {
		switch name {
		case "source", "version", "count", "for_each", "providers", "depends_on":
			continue
		}
		args[name] = a.Expr
	}
	for name, def := range mod.Variables {
		if expr, ok := args[name]; ok {
			vars[name] = ev.eval(expr, in, nil)
		} else if def != nil {
			vars[name] = ev.eval(def, nil, nil)
		} else {
			vars[name] = cty.DynamicVal
		}
	}
	child := &instance{mod: mod, path: key, parent: in, callArgs: args, vars: vars}
	ev.evalInstance(child)
	prev, had := in.children[call.Name]
	in.children[call.Name] = child
	if !had {
		return true
	}
	return !cty.ObjectVal(orEmpty(prev.outputs)).RawEquals(cty.ObjectVal(orEmpty(child.outputs)))
}

func orEmpty(m map[string]cty.Value) map[string]cty.Value {
	if m == nil {
		return map[string]cty.Value{}
	}
	return m
}

// instances evaluates count / for_each: -1 unknown, otherwise the size.
func (ev *evaluator) instances(count, forEach hcl.Expression, in *instance) int {
	if count != nil {
		v := ev.eval(count, in, nil)
		if !v.IsWhollyKnown() || v.IsNull() || !v.Type().Equals(cty.Number) {
			return -1
		}
		f, _ := v.AsBigFloat().Int64()
		return int(f)
	}
	if forEach != nil {
		v := ev.eval(forEach, in, nil)
		if !v.IsKnown() || v.IsNull() || !v.CanIterateElements() {
			return -1
		}
		return v.LengthInt()
	}
	return 1
}

func (ev *evaluator) ctx(in *instance, extra map[string]cty.Value) *hcl.EvalContext {
	vars := map[string]cty.Value{
		"count":     cty.ObjectVal(map[string]cty.Value{"index": cty.NumberIntVal(0)}),
		"each":      cty.ObjectVal(map[string]cty.Value{"key": cty.DynamicVal, "value": cty.DynamicVal}),
		"self":      cty.DynamicVal,
		"terraform": cty.ObjectVal(map[string]cty.Value{"workspace": cty.StringVal("default")}),
	}
	if in != nil {
		vars["var"] = cty.ObjectVal(orEmpty(in.vars))
		vars["local"] = cty.ObjectVal(orEmpty(in.locals))
		vars["path"] = cty.ObjectVal(map[string]cty.Value{
			"module": cty.StringVal(in.mod.Dir), "root": cty.StringVal(ev.rootDir), "cwd": cty.StringVal(ev.rootDir),
		})
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
		vars["module"] = cty.ObjectVal(mods)
		managed := map[string]map[string]cty.Value{}
		data := map[string]map[string]cty.Value{}
		for _, r := range in.mod.Resources {
			target := managed
			if r.Mode == "data" {
				target = data
			}
			if target[r.Type] == nil {
				target[r.Type] = map[string]cty.Value{}
			}
			target[r.Type][r.Name] = cty.DynamicVal
		}
		for t, names := range managed {
			vars[t] = cty.ObjectVal(names)
		}
		dataObj := map[string]cty.Value{}
		for t, names := range data {
			dataObj[t] = cty.ObjectVal(names)
		}
		vars["data"] = cty.ObjectVal(dataObj)
	}
	for k, v := range extra {
		vars[k] = v
	}
	return &hcl.EvalContext{Variables: vars, Functions: ev.funcs}
}

// eval never fails: errors become unknown values, and unknown functions are reported once.
func (ev *evaluator) eval(expr hcl.Expression, in *instance, extra map[string]cty.Value) cty.Value {
	v, diags := expr.Value(ev.ctx(in, extra))
	if diags.HasErrors() {
		for _, d := range diags {
			if strings.HasPrefix(d.Summary, "Call to unknown function") {
				ev.warn("%s: %s", d.Summary, d.Detail)
			}
		}
		return cty.DynamicVal
	}
	return v
}

// --- references --------------------------------------------------------------

func (ev *evaluator) refs(expr hcl.Expression, in *instance) []string {
	set := map[string]bool{}
	for _, t := range expr.Variables() {
		for _, a := range ev.resolve(t, in) {
			set[a] = true
		}
	}
	return sortedKeys(set)
}

func (ev *evaluator) resolve(t hcl.Traversal, in *instance) []string {
	root := t.RootName()
	name := step(t, 1)
	switch root {
	case "count", "each", "path", "terraform", "self":
		return nil
	case "var":
		if in.parent == nil || name == "" {
			return nil
		}
		expr, ok := in.callArgs[name]
		if !ok {
			return nil
		}
		return ev.memoRefs(in.addr()+"var."+name, expr, in.parent)
	case "local":
		expr, ok := in.mod.Locals[name]
		if !ok {
			return nil
		}
		return ev.memoRefs(in.addr()+"local."+name, expr, in)
	case "module":
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
	case "data":
		typ, n := name, step(t, 2)
		if in.mod.has("data", typ, n) {
			return []string{in.addr() + "data." + typ + "." + n}
		}
		return nil
	}
	if in.mod.has("managed", root, name) {
		return []string{in.addr() + root + "." + name}
	}
	return nil
}

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

func (m *Module) has(mode, typ, name string) bool {
	for _, r := range m.Resources {
		if r.Mode == mode && r.Type == typ && r.Name == name {
			return true
		}
	}
	return false
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

// --- resources ---------------------------------------------------------------

func (ev *evaluator) collect(in *instance, out *Evaluated) {
	for _, rb := range in.mod.Resources {
		n := ev.instances(rb.Count, rb.ForEach, in)
		if n == 0 {
			out.Removed = append(out.Removed, &Resource{Address: resourceAddr(in.addr(), rb), Mode: rb.Mode, Type: rb.Type, Name: rb.Name})
			continue
		}
		extra := map[string]cty.Value{}
		if rb.ForEach != nil {
			if v := ev.eval(rb.ForEach, in, nil); v.IsWhollyKnown() && !v.IsNull() && v.CanIterateElements() && v.LengthInt() > 0 {
				it := v.ElementIterator()
				it.Next()
				k, e := it.Element()
				if v.Type().IsSetType() {
					k = e
				}
				extra["each"] = cty.ObjectVal(map[string]cty.Value{"key": k, "value": e})
			}
		}
		addr := resourceAddr(in.addr(), rb)
		r := &Resource{
			Address: addr, Mode: rb.Mode, Type: rb.Type, Name: rb.Name, Module: strings.TrimSuffix(in.addr(), "."),
			Attrs: ev.body(rb.Body, in, extra), Refs: map[string][]string{}, Instances: n,
		}
		ev.bodyRefs(rb.Body, in, "", r.Refs)
		r.Statements = ev.statements(rb, in, extra)
		out.Resources = append(out.Resources, r)
	}
	names := make([]string, 0, len(in.children))
	for name := range in.children {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		ev.collect(in.children[name], out)
	}
	for _, call := range in.mod.Calls {
		if in.removed[call.Name] {
			ev.collectRemoved(in, call, out)
		}
	}
}

// collectRemoved lists the resources of a module call that is off, so the
// user can see what the current variables leave out.
func (ev *evaluator) collectRemoved(in *instance, call *ModuleCall, out *Evaluated) {
	key := append(append([]string(nil), in.path...), call.Name)
	dir, err := ev.ld.resolve(in.mod.Dir, call, key)
	if err != nil {
		return
	}
	mod, err := ev.ld.load(dir)
	if err != nil {
		return
	}
	child := &instance{mod: mod, path: key}
	for _, rb := range mod.Resources {
		out.Removed = append(out.Removed, &Resource{Address: resourceAddr(child.addr(), rb), Mode: rb.Mode, Type: rb.Type, Name: rb.Name})
	}
}

func resourceAddr(prefix string, rb *ResourceBlock) string {
	if rb.Mode == "data" {
		return prefix + "data." + rb.Type + "." + rb.Name
	}
	return prefix + rb.Type + "." + rb.Name
}

var skipBlocks = map[string]bool{"lifecycle": true, "provisioner": true, "connection": true}

func (ev *evaluator) body(b *hclsyntax.Body, in *instance, extra map[string]cty.Value) map[string]any {
	out := map[string]any{}
	for name, a := range b.Attributes {
		if name == "count" || name == "for_each" || name == "depends_on" || name == "provider" {
			continue
		}
		if v := toGo(ev.eval(a.Expr, in, extra)); v != nil {
			out[name] = v
		}
	}
	for _, blk := range b.Blocks {
		if skipBlocks[blk.Type] {
			continue
		}
		name, body, ext := blk.Type, blk.Body, extra
		if blk.Type == "dynamic" && len(blk.Labels) == 1 {
			name = blk.Labels[0]
			content := dynamicContent(blk)
			if content == nil {
				continue
			}
			body = content
			ext = withIterator(extra, blk)
		}
		list, _ := out[name].([]any)
		out[name] = append(list, ev.body(body, in, ext))
	}
	return out
}

func dynamicContent(blk *hclsyntax.Block) *hclsyntax.Body {
	for _, c := range blk.Body.Blocks {
		if c.Type == "content" {
			return c.Body
		}
	}
	return nil
}

func withIterator(extra map[string]cty.Value, blk *hclsyntax.Block) map[string]cty.Value {
	it := iteratorName(blk)
	out := map[string]cty.Value{}
	for k, v := range extra {
		out[k] = v
	}
	out[it] = cty.ObjectVal(map[string]cty.Value{"key": cty.DynamicVal, "value": cty.DynamicVal})
	return out
}

func (ev *evaluator) bodyRefs(b *hclsyntax.Body, in *instance, prefix string, out map[string][]string) {
	for name, a := range b.Attributes {
		if name == "count" || name == "for_each" || name == "depends_on" || name == "provider" {
			continue
		}
		if r := ev.refs(a.Expr, in); len(r) > 0 {
			out[prefix+name] = union(out[prefix+name], r)
		}
	}
	for _, blk := range b.Blocks {
		if skipBlocks[blk.Type] {
			continue
		}
		if blk.Type == "dynamic" && len(blk.Labels) == 1 {
			body, fe := dynamicContent(blk), blk.Body.Attributes["for_each"]
			if body == nil || fe == nil {
				continue
			}
			iter := iteratorName(blk)
			for name, a := range body.Attributes {
				if r := ev.iterRefs(a.Expr, in, iter, fe.Expr, pathStep{index: -2}); len(r) > 0 {
					p := prefix + blk.Labels[0] + "." + name
					out[p] = union(out[p], r)
				}
			}
			continue
		}
		ev.bodyRefs(blk.Body, in, prefix+blk.Type+".", out)
	}
}

func (ev *evaluator) region(root *instance) string {
	for _, p := range root.mod.Providers {
		if len(p.Labels) != 1 || p.Labels[0] != "aws" {
			continue
		}
		if _, aliased := p.Body.Attributes["alias"]; aliased {
			continue
		}
		if a, ok := p.Body.Attributes["region"]; ok {
			if v := ev.eval(a.Expr, root, nil); v.IsKnown() && !v.IsNull() && v.Type() == cty.String {
				return v.AsString()
			}
		}
	}
	return ""
}

// --- variables ---------------------------------------------------------------

func rootVars(m *Module, dir string, opt Options) (map[string]cty.Value, error) {
	vars := map[string]cty.Value{}
	for name, def := range m.Variables {
		vars[name] = cty.DynamicVal
		if def != nil {
			if v, diags := def.Value(nil); !diags.HasErrors() {
				vars[name] = v
			}
		}
	}
	files := []string{}
	for _, f := range []string{"terraform.tfvars", "terraform.tfvars.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			files = append(files, filepath.Join(dir, f))
		}
	}
	auto, _ := filepath.Glob(filepath.Join(dir, "*.auto.tfvars"))
	autoJSON, _ := filepath.Glob(filepath.Join(dir, "*.auto.tfvars.json"))
	sort.Strings(auto)
	sort.Strings(autoJSON)
	files = append(append(append(files, auto...), autoJSON...), opt.VarFiles...)
	p := hclparse.NewParser()
	for _, path := range files {
		var f *hcl.File
		var diags hcl.Diagnostics
		if strings.HasSuffix(path, ".json") {
			f, diags = p.ParseJSONFile(path)
		} else {
			f, diags = p.ParseHCLFile(path)
		}
		if diags.HasErrors() {
			return nil, fmt.Errorf("%s", diags.Error())
		}
		attrs, diags := f.Body.JustAttributes()
		if diags.HasErrors() {
			return nil, fmt.Errorf("%s", diags.Error())
		}
		for name, a := range attrs {
			if v, diags := a.Expr.Value(nil); !diags.HasErrors() {
				vars[name] = v
			}
		}
	}
	for name, raw := range opt.Vars {
		expr, diags := hclsyntax.ParseExpression([]byte(raw), "-var", hcl.InitialPos)
		v := cty.StringVal(raw)
		if !diags.HasErrors() {
			if parsed, d := expr.Value(nil); !d.HasErrors() {
				v = parsed
			}
		}
		vars[name] = v
	}
	return vars, nil
}

// --- conversion --------------------------------------------------------------

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
		f, _ := v.AsBigFloat().Float64()
		if v.AsBigFloat().IsInt() {
			i, acc := v.AsBigFloat().Int64()
			if acc == big.Exact {
				return int(i)
			}
		}
		return f
	case t == cty.Bool:
		return v.True()
	case t.IsListType() || t.IsTupleType() || t.IsSetType():
		out := []any{}
		for it := v.ElementIterator(); it.Next(); {
			_, e := it.Element()
			if g := toGo(e); g != nil {
				out = append(out, g)
			}
		}
		return out
	case t.IsMapType() || t.IsObjectType():
		out := map[string]any{}
		for it := v.ElementIterator(); it.Next(); {
			k, e := it.Element()
			if g := toGo(e); g != nil {
				out[k.AsString()] = g
			}
		}
		return out
	}
	return nil
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
