package eval

import (
	"github.com/O6lvl4/archgopher/terraform/config"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// Resources name each other: an Aurora instance takes its engine from
// `aws_rds_cluster.db.engine`, a policy names a bucket's `bucket`. Terraform
// knows such a value at plan time when the referenced block sets it in the
// configuration; only attributes the provider fills in after apply (`arn`,
// `id`, `endpoint`) are unknown then. The evaluator does the same: a managed
// resource that is not counted is the object of its configured arguments, and
// a reference to an argument it does not set reads unknown, as before.

// initResources gives every managed, uncounted resource of an instance an
// unknown value, before the first pass settles them.
func (in *instance) initResources() {
	in.resources = map[string]cty.Value{}
	for _, rb := range in.mod.Resources {
		if settled(rb) {
			in.resources[rb.Type+"."+rb.Name] = cty.DynamicVal
		}
	}
}

// settled says whether a resource block's configured arguments are read as
// its value: a managed block without count or for_each. A counted block is a
// list or map of instances; its index is seldom a literal, so it stays unknown.
func settled(rb *config.ResourceBlock) bool {
	return rb.Mode == "managed" && rb.Count == nil && rb.ForEach == nil
}

// settleResources evaluates the configured arguments of every settled
// resource once and reports whether any value changed. Arguments can name
// locals and other resources, so evalInstance repeats it with the locals
// until nothing moves.
func (ev *evaluator) settleResources(in *instance) bool {
	changed := false
	for _, rb := range in.mod.Resources {
		if !settled(rb) {
			continue
		}
		key := rb.Type + "." + rb.Name
		v := ev.bodyValue(rb.Body, in, nil)
		if !v.RawEquals(in.resources[key]) {
			in.resources[key], changed = v, true
		}
	}
	return changed
}

// bodyValue is a block as the object Terraform exposes it as: its arguments
// by name and its nested blocks as tuples of objects, so `x.block[0].attr`
// reads. Meta-arguments and lifecycle blocks are not part of the value.
func (ev *evaluator) bodyValue(b *hclsyntax.Body, in *instance, extra map[string]cty.Value) cty.Value {
	attrs := map[string]cty.Value{}
	for name, a := range b.Attributes {
		if metaArgs[name] {
			continue
		}
		attrs[name] = ev.eval(a.Expr, in, extra)
	}
	lists := map[string][]cty.Value{}
	for _, blk := range b.Blocks {
		name, body, ext, ok := nestedBlock(blk, extra)
		if !ok {
			continue
		}
		lists[name] = append(lists[name], ev.bodyValue(body, in, ext))
	}
	for name, vals := range lists {
		attrs[name] = cty.TupleVal(vals)
	}
	return cty.ObjectVal(attrs)
}
