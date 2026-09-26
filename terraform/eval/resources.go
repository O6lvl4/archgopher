package eval

import (
	"sort"
	"strings"

	"github.com/O6lvl4/archgopher/terraform/config"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// collect evaluates the resources of an instance and, depth first in name
// order, of its children, and lists what module calls that are off leave out.
func (ev *evaluator) collect(in *instance, out *Evaluated) {
	for _, rb := range in.mod.Resources {
		ev.collectResource(in, rb, out)
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

// collectResource evaluates one resource block as its first instance, or
// lists it as removed when count or for_each is 0.
func (ev *evaluator) collectResource(in *instance, rb *config.ResourceBlock, out *Evaluated) {
	n := ev.instances(rb.Count, rb.ForEach, in)
	if n == 0 {
		out.Removed = append(out.Removed, removedResource(in.addr(), rb))
		return
	}
	extra := map[string]cty.Value{}
	if each, ok := ev.firstEach(rb.ForEach, in); ok {
		extra["each"] = each
	}
	r := &Resource{
		Address: resourceAddr(in.addr(), rb), Mode: rb.Mode, Type: rb.Type, Name: rb.Name, Module: strings.TrimSuffix(in.addr(), "."),
		Attrs: ev.body(rb.Body, in, extra), Refs: map[string][]string{}, Instances: times(in.copies, n),
	}
	ev.bodyRefs(rb.Body, in, "", r.Refs)
	r.Body, r.Scope = rb.Body, &Scope{ev: ev, in: in, extra: extra}
	out.Resources = append(out.Resources, r)
}

// firstEach is the each object of the first element of a known, non-empty
// for_each collection. A set's key is its value, as in Terraform.
func (ev *evaluator) firstEach(forEach hcl.Expression, in *instance) (cty.Value, bool) {
	if forEach == nil {
		return cty.NilVal, false
	}
	v := ev.eval(forEach, in, nil)
	if !v.IsWhollyKnown() || v.IsNull() || !v.CanIterateElements() || v.LengthInt() == 0 {
		return cty.NilVal, false
	}
	it := v.ElementIterator()
	it.Next()
	k, e := it.Element()
	if v.Type().IsSetType() {
		k = e
	}
	return cty.ObjectVal(map[string]cty.Value{"key": k, "value": e}), true
}

// collectRemoved lists the resources of a module call that is off, so the
// user can see what the current variables leave out.
func (ev *evaluator) collectRemoved(in *instance, call *config.ModuleCall, out *Evaluated) {
	key, mod, err := ev.loadCall(in, call)
	if err != nil {
		return
	}
	child := &instance{mod: mod, path: key, copies: 1}
	for _, rb := range mod.Resources {
		out.Removed = append(out.Removed, removedResource(child.addr(), rb))
	}
}

func removedResource(prefix string, rb *config.ResourceBlock) *Resource {
	return &Resource{Address: resourceAddr(prefix, rb), Mode: rb.Mode, Type: rb.Type, Name: rb.Name}
}

func resourceAddr(prefix string, rb *config.ResourceBlock) string {
	return address(prefix, rb.Mode, rb.Type, rb.Name)
}

// address is the Terraform address of a block under a module prefix.
func address(prefix, mode, typ, name string) string {
	if mode == "data" {
		return prefix + "data." + typ + "." + name
	}
	return prefix + typ + "." + name
}

// metaArgs are resource arguments Terraform reads itself; they are neither
// attributes of the resource nor sources of its edges.
var metaArgs = map[string]bool{"count": true, "for_each": true, "depends_on": true, "provider": true}

var skipBlocks = map[string]bool{"lifecycle": true, "provisioner": true, "connection": true}

// body evaluates the attributes and nested blocks of a block. Nested blocks,
// including the content of dynamic blocks, become lists of maps.
func (ev *evaluator) body(b *hclsyntax.Body, in *instance, extra map[string]cty.Value) map[string]any {
	out := map[string]any{}
	for name, a := range b.Attributes {
		if metaArgs[name] {
			continue
		}
		if v := toGo(ev.eval(a.Expr, in, extra)); v != nil {
			out[name] = v
		}
	}
	for _, blk := range b.Blocks {
		name, body, ext, ok := nestedBlock(blk, extra)
		if !ok {
			continue
		}
		list, _ := out[name].([]any)
		out[name] = append(list, ev.body(body, in, ext))
	}
	return out
}

// nestedBlock returns the name a nested block is stored under, the body to
// evaluate and the variables in effect there. A dynamic block contributes its
// content with its iterator unknown; ok is false for blocks that carry no
// attributes of the resource.
func nestedBlock(blk *hclsyntax.Block, extra map[string]cty.Value) (string, *hclsyntax.Body, map[string]cty.Value, bool) {
	if skipBlocks[blk.Type] {
		return "", nil, nil, false
	}
	if blk.Type != "dynamic" || len(blk.Labels) != 1 {
		return blk.Type, blk.Body, extra, true
	}
	content := dynamicContent(blk)
	if content == nil {
		return "", nil, nil, false
	}
	return blk.Labels[0], content, withIterator(extra, blk), true
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

// bodyRefs records, by attribute path, the resources each attribute of a
// block and its nested blocks references.
func (ev *evaluator) bodyRefs(b *hclsyntax.Body, in *instance, prefix string, out map[string][]string) {
	for name, a := range b.Attributes {
		if metaArgs[name] {
			continue
		}
		addRefs(out, prefix+name, ev.refs(a.Expr, in))
	}
	for _, blk := range b.Blocks {
		switch {
		case skipBlocks[blk.Type]:
		case blk.Type == "dynamic" && len(blk.Labels) == 1:
			ev.dynamicRefs(blk, in, prefix, out)
		default:
			ev.bodyRefs(blk.Body, in, prefix+blk.Type+".", out)
		}
	}
}

// dynamicRefs records the references of a dynamic block's content, with the
// iterator standing for every element of its for_each.
func (ev *evaluator) dynamicRefs(blk *hclsyntax.Block, in *instance, prefix string, out map[string][]string) {
	body, fe := dynamicContent(blk), blk.Body.Attributes["for_each"]
	if body == nil || fe == nil {
		return
	}
	it := iterator{name: iteratorName(blk), forEach: fe.Expr, elem: Every}
	for name, a := range body.Attributes {
		addRefs(out, prefix+blk.Labels[0]+"."+name, ev.iterRefs(a.Expr, in, it))
	}
}

// addRefs merges refs into the entry for path; an empty list adds no entry.
func addRefs(out map[string][]string, path string, refs []string) {
	if len(refs) > 0 {
		out[path] = union(out[path], refs)
	}
}

// providers evaluates the unaliased provider blocks of the root module.
func (ev *evaluator) providers(root *instance) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, p := range root.mod.Providers {
		if len(p.Labels) != 1 {
			continue
		}
		if _, aliased := p.Body.Attributes["alias"]; aliased {
			continue
		}
		out[p.Labels[0]] = ev.body(p.Body, root, nil)
	}
	return out
}
