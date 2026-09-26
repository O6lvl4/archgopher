// Package iam reads IAM policies out of Terraform and turns the permissions a
// node's role holds into edges with kinds of work: dynamodb:PutItem on a table
// is a write edge. It plugs into terraform/infer as an edge source.
package iam

import (
	"encoding/json"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/O6lvl4/archgopher/terraform/eval"
)

const policyDocument = "aws_iam_policy_document"

// Statement is one Allow statement of an IAM policy.
type Statement struct {
	// Actions is nil when the actions could not be read.
	Actions []string
	Targets []string
	// Docs are policy documents (data sources) the statement defers to.
	Docs []string
}

// Statements reads the Allow statements of a resource. It works on syntax,
// because resource ARNs are unknown before apply: the actions are literals and
// the resources are references, which is exactly what edges need.
func Statements(r *eval.Resource) []Statement {
	if r.Body == nil || r.Scope == nil {
		return nil
	}
	if r.Mode == "data" && r.Type == policyDocument {
		return documentStatements(r.Body, r.Scope)
	}
	var exprs []hcl.Expression
	if a, ok := r.Body.Attributes["policy"]; ok {
		exprs = append(exprs, a.Expr)
	}
	for _, blk := range blocksNamed(r.Body, "inline_policy") {
		if a, ok := blk.Attributes["policy"]; ok {
			exprs = append(exprs, a.Expr)
		}
	}
	var out []Statement
	for _, e := range exprs {
		out = append(out, policy(e, r.Scope)...)
	}
	return out
}

func documentStatements(body *hclsyntax.Body, s *eval.Scope) []Statement {
	var out []Statement
	for _, blk := range body.Blocks {
		switch {
		case blk.Type == "statement":
			out = append(out, docStatement(blk.Body, s, s.Refs)...)
		case blk.Type == "dynamic" && len(blk.Labels) == 1 && blk.Labels[0] == "statement":
			out = append(out, dynamicStatements(blk, s)...)
		}
	}
	return out
}

// docStatement reads one statement block; refsOf resolves its resources.
func docStatement(body *hclsyntax.Body, s *eval.Scope, refsOf func(hcl.Expression) []string) []Statement {
	if a, ok := body.Attributes["effect"]; ok && strings.EqualFold(eval.String(s.Eval(a.Expr)), "Deny") {
		return nil
	}
	st := Statement{}
	if a, ok := body.Attributes["actions"]; ok {
		st.Actions = eval.Strings(s.Eval(a.Expr))
	}
	if a, ok := body.Attributes["resources"]; ok {
		st.Targets = refsOf(a.Expr)
	}
	return []Statement{st}
}

// dynamicStatements expands `dynamic "statement"` element by element, so each
// statement keeps its own actions and resources.
func dynamicStatements(blk *hclsyntax.Block, s *eval.Scope) []Statement {
	content := eval.DynamicContent(blk)
	fe, ok := blk.Body.Attributes["for_each"]
	if content == nil || !ok {
		return nil
	}
	iter := eval.IteratorName(blk)
	elems, known := s.Elements(fe.Expr)
	if !known {
		// Unknown collection: one statement with every reference and no actions.
		return docStatement(content, s, func(e hcl.Expression) []string { return s.IterRefs(e, iter, fe.Expr, eval.Every) })
	}
	var out []Statement
	for _, el := range elems {
		inner := s.With(iter, eval.IteratorValue(el))
		step := el.Step
		out = append(out, docStatement(content, inner, func(x hcl.Expression) []string { return s.IterRefs(x, iter, fe.Expr, step) })...)
	}
	return out
}

// encodedPolicy reads a policy written as jsonencode({...}); it reports
// false for any other expression, or an object it cannot read.
func encodedPolicy(expr hcl.Expression, s *eval.Scope) ([]Statement, bool) {
	call, ok := expr.(*hclsyntax.FunctionCallExpr)
	if !ok || call.Name != "jsonencode" || len(call.Args) != 1 {
		return nil, false
	}
	return policyObject(call.Args[0], s)
}

func policy(expr hcl.Expression, s *eval.Scope) []Statement {
	if sts, ok := encodedPolicy(expr, s); ok {
		return sts
	}
	var docs, targets []string
	for _, r := range s.Refs(expr) {
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
		out = append(out, Statement{Actions: literalActions(eval.String(s.Eval(expr))), Targets: targets})
	}
	return out
}

// policyObject reads jsonencode({ Statement = [...] }).
func policyObject(doc hclsyntax.Expression, s *eval.Scope) ([]Statement, bool) {
	obj, ok := doc.(*hclsyntax.ObjectConsExpr)
	if !ok {
		return nil, false
	}
	var list []hclsyntax.Expression
	for _, item := range obj.Items {
		if eval.KeyName(item.KeyExpr) != "Statement" {
			continue
		}
		if tuple, ok := item.ValueExpr.(*hclsyntax.TupleConsExpr); ok {
			list = tuple.Exprs
		} else {
			list = []hclsyntax.Expression{item.ValueExpr}
		}
	}
	var out []Statement
	for _, e := range list {
		if st, ok := objectStatement(e, s); ok {
			out = append(out, st)
		}
	}
	return out, true
}

func objectStatement(e hclsyntax.Expression, s *eval.Scope) (Statement, bool) {
	obj, ok := e.(*hclsyntax.ObjectConsExpr)
	if !ok {
		return Statement{Targets: s.Refs(e)}, true
	}
	st := Statement{}
	for _, item := range obj.Items {
		switch eval.KeyName(item.KeyExpr) {
		case "Effect":
			if strings.EqualFold(eval.String(s.Eval(item.ValueExpr)), "Deny") {
				return Statement{}, false
			}
		case "Action":
			st.Actions = eval.Strings(s.Eval(item.ValueExpr))
		case "Resource":
			st.Targets = s.Refs(item.ValueExpr)
		}
	}
	return st, true
}

// literalActions reads actions from a policy string when it is fully known.
func literalActions(text string) []string {
	if text == "" {
		return nil
	}
	var doc struct {
		Statement []struct {
			Effect string
			Action any
		}
	}
	if json.Unmarshal([]byte(text), &doc) != nil {
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
			if c := eval.DynamicContent(blk); c != nil {
				out = append(out, c)
			}
		}
	}
	return out
}
