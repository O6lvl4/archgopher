package terraform

import (
	"encoding/json"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

const policyDocument = "aws_iam_policy_document"

// statements reads the Allow statements of a resource's IAM policies. It works
// on syntax, because resource ARNs are unknown before apply: the actions are
// literals and the resources are references, which is exactly what edges need.
func (ev *evaluator) statements(rb *ResourceBlock, in *instance, extra map[string]cty.Value) []Statement {
	if rb.Mode == "data" && rb.Type == policyDocument {
		var out []Statement
		for _, blk := range rb.Body.Blocks {
			switch {
			case blk.Type == "statement":
				out = append(out, ev.docStatement(blk.Body, in, extra, nil)...)
			case blk.Type == "dynamic" && len(blk.Labels) == 1 && blk.Labels[0] == "statement":
				out = append(out, ev.dynamicStatements(blk, in, extra)...)
			}
		}
		return out
	}
	var exprs []hcl.Expression
	if a, ok := rb.Body.Attributes["policy"]; ok {
		exprs = append(exprs, a.Expr)
	}
	for _, blk := range blocksNamed(rb.Body, "inline_policy") {
		if a, ok := blk.Attributes["policy"]; ok {
			exprs = append(exprs, a.Expr)
		}
	}
	var out []Statement
	for _, e := range exprs {
		out = append(out, ev.policy(e, in, extra)...)
	}
	return out
}

func (ev *evaluator) policy(expr hcl.Expression, in *instance, extra map[string]cty.Value) []Statement {
	if call, ok := expr.(*hclsyntax.FunctionCallExpr); ok && call.Name == "jsonencode" && len(call.Args) == 1 {
		if sts, ok := ev.policyObject(call.Args[0], in, extra); ok {
			return sts
		}
	}
	var docs, targets []string
	for _, r := range ev.refs(expr, in) {
		if strings.Contains(r, "data."+policyDocument+".") {
			docs = append(docs, r)
		} else {
			targets = append(targets, r)
		}
	}
	var out []Statement
	if len(docs) > 0 {
		out = append(out, Statement{Docs: docs})
	}
	if len(targets) > 0 {
		out = append(out, Statement{Actions: literalActions(ev.eval(expr, in, extra)), Targets: targets})
	}
	return out
}

// policyObject reads jsonencode({ Statement = [...] }).
func (ev *evaluator) policyObject(doc hclsyntax.Expression, in *instance, extra map[string]cty.Value) ([]Statement, bool) {
	obj, ok := doc.(*hclsyntax.ObjectConsExpr)
	if !ok {
		return nil, false
	}
	var list []hclsyntax.Expression
	for _, item := range obj.Items {
		if keyName(item.KeyExpr) != "Statement" {
			continue
		}
		switch v := item.ValueExpr.(type) {
		case *hclsyntax.TupleConsExpr:
			list = v.Exprs
		default:
			list = []hclsyntax.Expression{v}
		}
	}
	var out []Statement
	for _, e := range list {
		st, ok := e.(*hclsyntax.ObjectConsExpr)
		if !ok {
			out = append(out, Statement{Targets: ev.refs(e, in)})
			continue
		}
		s, deny := Statement{}, false
		for _, item := range st.Items {
			switch keyName(item.KeyExpr) {
			case "Effect":
				deny = strings.EqualFold(stringOf(ev.eval(item.ValueExpr, in, extra)), "Deny")
			case "Action":
				s.Actions = toStrings(ev.eval(item.ValueExpr, in, extra))
			case "Resource":
				s.Targets = ev.refs(item.ValueExpr, in)
			}
		}
		if !deny {
			out = append(out, s)
		}
	}
	return out, true
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

// literalActions reads actions from a policy string when it is fully known.
func literalActions(v cty.Value) []string {
	s := stringOf(v)
	if s == "" {
		return nil
	}
	var doc struct {
		Statement []struct {
			Effect string
			Action any
		}
	}
	if json.Unmarshal([]byte(s), &doc) != nil {
		return nil
	}
	var out []string
	for _, st := range doc.Statement {
		if strings.EqualFold(st.Effect, "Deny") {
			continue
		}
		switch a := st.Action.(type) {
		case string:
			out = append(out, a)
		case []any:
			for _, x := range a {
				if s, ok := x.(string); ok {
					out = append(out, s)
				}
			}
		}
	}
	return out
}

func blocksNamed(b *hclsyntax.Body, name string) []*hclsyntax.Body {
	var out []*hclsyntax.Body
	for _, blk := range b.Blocks {
		switch {
		case blk.Type == name:
			out = append(out, blk.Body)
		case blk.Type == "dynamic" && len(blk.Labels) == 1 && blk.Labels[0] == name:
			if c := dynamicContent(blk); c != nil {
				out = append(out, c)
			}
		}
	}
	return out
}

func stringOf(v cty.Value) string {
	if !v.IsKnown() || v.IsNull() || v.Type() != cty.String {
		return ""
	}
	return v.AsString()
}

func toStrings(v cty.Value) []string {
	g := toGo(v)
	switch x := g.(type) {
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

// docStatement reads one statement block. refsOf overrides reference
// resolution inside a dynamic block; nil means plain resolution.
func (ev *evaluator) docStatement(body *hclsyntax.Body, in *instance, extra map[string]cty.Value, refsOf func(hcl.Expression) []string) []Statement {
	if a, ok := body.Attributes["effect"]; ok && strings.EqualFold(stringOf(ev.eval(a.Expr, in, extra)), "Deny") {
		return nil
	}
	if refsOf == nil {
		refsOf = func(e hcl.Expression) []string { return ev.refs(e, in) }
	}
	st := Statement{}
	if a, ok := body.Attributes["actions"]; ok {
		st.Actions = toStrings(ev.eval(a.Expr, in, extra))
	}
	if a, ok := body.Attributes["resources"]; ok {
		st.Targets = refsOf(a.Expr)
	}
	return []Statement{st}
}

// dynamicStatements expands `dynamic "statement"` element by element, so each
// statement keeps its own actions and resources.
func (ev *evaluator) dynamicStatements(blk *hclsyntax.Block, in *instance, extra map[string]cty.Value) []Statement {
	content := dynamicContent(blk)
	fe, ok := blk.Body.Attributes["for_each"]
	if content == nil || !ok {
		return nil
	}
	iter := iteratorName(blk)
	v := ev.eval(fe.Expr, in, extra)
	if !v.IsKnown() || v.IsNull() || !v.CanIterateElements() {
		// Unknown collection: one statement with every reference and no actions.
		return ev.docStatement(content, in, withIterator(extra, blk), func(e hcl.Expression) []string {
			return ev.iterRefs(e, in, iter, fe.Expr, pathStep{index: -2})
		})
	}
	var out []Statement
	i := 0
	for it := v.ElementIterator(); it.Next(); i++ {
		k, e := it.Element()
		ext := map[string]cty.Value{}
		for name, val := range extra {
			ext[name] = val
		}
		ext[iter] = cty.ObjectVal(map[string]cty.Value{"key": k, "value": e})
		elem := pathStep{index: i}
		if t := v.Type(); t.IsMapType() || t.IsObjectType() {
			elem = pathStep{attr: k.AsString(), index: -1}
		}
		out = append(out, ev.docStatement(content, in, ext, func(x hcl.Expression) []string {
			return ev.iterRefs(x, in, iter, fe.Expr, elem)
		})...)
	}
	return out
}

func iteratorName(blk *hclsyntax.Block) string {
	if a, ok := blk.Body.Attributes["iterator"]; ok {
		if t, diags := hcl.AbsTraversalForExpr(a.Expr); !diags.HasErrors() {
			return t.RootName()
		}
	}
	return blk.Labels[0]
}
