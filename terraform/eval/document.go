package eval

import (
	"encoding/json"
	"io/fs"
	"path"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
)

// Unknown stands in a rendered document for a value known only after apply.
const Unknown = "(known after apply)"

// maxDocumentHops bounds the variables, locals and module outputs a document
// is followed through.
const maxDocumentHops = 8

// Document reads an attribute holding a JSON document, such as a state
// machine's definition, for its structure. The structure is written before
// apply even when values inside it are not: one ARN makes a whole
// jsonencode() unknown to Eval. Document reads jsonencode's argument instead
// and keeps its known parts; it renders templates (inline, heredoc, file and
// templatefile from the configuration's files) with each interpolation known
// only after apply written as Unknown; and it follows variables, locals and
// module outputs. ok is false when no document can be read.
func (s *Scope) Document(expr hcl.Expression) (any, bool) {
	return s.ev.document(expr, s.in, s.extra, 0)
}

func (ev *evaluator) document(expr hcl.Expression, in *instance, extra map[string]cty.Value, depth int) (any, bool) {
	if depth > maxDocumentHops {
		return nil, false
	}
	switch e := expr.(type) {
	case *hclsyntax.ScopeTraversalExpr:
		if target, tin, rest, ok := ev.definition(e.Traversal, in); ok && len(rest) == 0 {
			return ev.document(target, tin, nil, depth+1)
		}
	case *hclsyntax.TemplateWrapExpr:
		return ev.document(e.Wrapped, in, extra, depth+1)
	case *hclsyntax.TemplateExpr:
		return decodeJSON(render(e, ev.ctx(in, extra)))
	case *hclsyntax.FunctionCallExpr:
		return ev.documentCall(e, in, extra)
	}
	if s := String(ev.eval(expr, in, extra)); s != "" {
		return decodeJSON(s)
	}
	return nil, false
}

// documentCall reads the functions a document is commonly written with.
func (ev *evaluator) documentCall(e *hclsyntax.FunctionCallExpr, in *instance, extra map[string]cty.Value) (any, bool) {
	switch {
	case e.Name == "jsonencode" && len(e.Args) == 1:
		doc := toGo(ev.eval(e.Args[0], in, extra))
		return doc, doc != nil
	case e.Name == "file" && len(e.Args) == 1:
		if data, ok := ev.readFile(e.Args[0], in, extra); ok {
			return decodeJSON(data)
		}
	case e.Name == "templatefile" && len(e.Args) == 2:
		data, ok := ev.readFile(e.Args[0], in, extra)
		if !ok {
			return nil, false
		}
		tmpl, diags := hclsyntax.ParseTemplate([]byte(data), "templatefile", hcl.InitialPos)
		vars := ev.eval(e.Args[1], in, extra)
		if diags.HasErrors() || !vars.IsKnown() || vars.IsNull() || !(vars.Type().IsObjectType() || vars.Type().IsMapType()) {
			return nil, false
		}
		return decodeJSON(render(tmpl, &hcl.EvalContext{Variables: vars.AsValueMap(), Functions: ev.funcs}))
	}
	if s := String(ev.eval(e, in, extra)); s != "" {
		return decodeJSON(s)
	}
	return nil, false
}

// readFile reads a file of the configuration. Terraform resolves a relative
// path against the working directory, the root module here; path.module and
// path.root are already paths inside the configuration's files.
func (ev *evaluator) readFile(arg hcl.Expression, in *instance, extra map[string]cty.Value) (string, bool) {
	name := String(ev.eval(arg, in, extra))
	if name == "" || ev.ld == nil || ev.ld.FS == nil {
		return "", false
	}
	for _, p := range []string{path.Clean(name), path.Join(ev.rootDir, name)} {
		if !fs.ValidPath(p) {
			continue
		}
		if data, err := fs.ReadFile(ev.ld.FS, p); err == nil {
			return string(data), true
		}
	}
	return "", false
}

// render renders a template part by part, writing Unknown for each part
// known only after apply.
func render(t hclsyntax.Expression, ctx *hcl.EvalContext) string {
	parts := []hclsyntax.Expression{t}
	if te, ok := t.(*hclsyntax.TemplateExpr); ok {
		parts = te.Parts
	}
	var b strings.Builder
	for _, p := range parts {
		v, _ := p.Value(ctx)
		v, _ = v.UnmarkDeep()
		if s, err := convert.Convert(v, cty.String); err == nil && s.IsKnown() && !s.IsNull() {
			b.WriteString(s.AsString())
		} else {
			b.WriteString(Unknown)
		}
	}
	return b.String()
}

func decodeJSON(s string) (any, bool) {
	var doc any
	if err := json.Unmarshal([]byte(s), &doc); err != nil {
		return nil, false
	}
	return doc, doc != nil
}
