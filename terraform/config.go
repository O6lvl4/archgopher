// Package terraform builds an arch-scouter declaration from Terraform source
// without running Terraform: it parses HCL, evaluates variables, locals,
// count and module wiring statically, and traces references between resources.
// Values that only exist after apply (ARNs, IDs) stay unknown; edges come from
// the references themselves, so they do not need those values.
package terraform

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// Module is one directory of .tf files.
type Module struct {
	Dir       string
	Variables map[string]hcl.Expression // default expressions; nil when no default
	Locals    map[string]hcl.Expression
	Resources []*ResourceBlock
	Calls     []*ModuleCall
	Outputs   map[string]hcl.Expression
	Providers []*hclsyntax.Block
}

// ResourceBlock is a resource or data block.
type ResourceBlock struct {
	Mode    string // "managed" or "data"
	Type    string
	Name    string
	Body    *hclsyntax.Body
	Count   hcl.Expression
	ForEach hcl.Expression
}

// ModuleCall is a module block.
type ModuleCall struct {
	Name    string
	Source  string
	Body    *hclsyntax.Body
	Count   hcl.Expression
	ForEach hcl.Expression
}

func parseModule(dir string) (*Module, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.tf"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no .tf files in %s", dir)
	}
	sort.Strings(files)
	m := &Module{Dir: dir, Variables: map[string]hcl.Expression{}, Locals: map[string]hcl.Expression{}, Outputs: map[string]hcl.Expression{}}
	p := hclparse.NewParser()
	for _, path := range files {
		f, diags := p.ParseHCLFile(path)
		if diags.HasErrors() {
			return nil, fmt.Errorf("%s", diags.Error())
		}
		body := f.Body.(*hclsyntax.Body)
		for _, b := range body.Blocks {
			m.addBlock(b)
		}
	}
	return m, nil
}

func (m *Module) addBlock(b *hclsyntax.Block) {
	switch b.Type {
	case "variable":
		if len(b.Labels) == 1 {
			var def hcl.Expression
			if a, ok := b.Body.Attributes["default"]; ok {
				def = a.Expr
			}
			m.Variables[b.Labels[0]] = def
		}
	case "locals":
		for name, a := range b.Body.Attributes {
			m.Locals[name] = a.Expr
		}
	case "resource", "data":
		if len(b.Labels) == 2 {
			mode := "managed"
			if b.Type == "data" {
				mode = "data"
			}
			m.Resources = append(m.Resources, &ResourceBlock{
				Mode: mode, Type: b.Labels[0], Name: b.Labels[1], Body: b.Body,
				Count: attrExpr(b.Body, "count"), ForEach: attrExpr(b.Body, "for_each"),
			})
		}
	case "module":
		if len(b.Labels) == 1 {
			src := ""
			if a, ok := b.Body.Attributes["source"]; ok {
				if v, diags := a.Expr.Value(nil); !diags.HasErrors() && v.Type().FriendlyName() == "string" {
					src = v.AsString()
				}
			}
			m.Calls = append(m.Calls, &ModuleCall{
				Name: b.Labels[0], Source: src, Body: b.Body,
				Count: attrExpr(b.Body, "count"), ForEach: attrExpr(b.Body, "for_each"),
			})
		}
	case "output":
		if len(b.Labels) == 1 {
			if a, ok := b.Body.Attributes["value"]; ok {
				m.Outputs[b.Labels[0]] = a.Expr
			}
		}
	case "provider":
		m.Providers = append(m.Providers, b)
	}
}

func attrExpr(body *hclsyntax.Body, name string) hcl.Expression {
	if a, ok := body.Attributes[name]; ok {
		return a.Expr
	}
	return nil
}

// loader resolves module sources and caches parsed modules.
type loader struct {
	root     string
	cache    map[string]*Module
	manifest map[string]string // module key ("a.b") -> directory, from terraform init
}

func newLoader(root string) *loader {
	l := &loader{root: root, cache: map[string]*Module{}, manifest: map[string]string{}}
	data, err := os.ReadFile(filepath.Join(root, ".terraform", "modules", "modules.json"))
	if err != nil {
		return l
	}
	var mf struct {
		Modules []struct{ Key, Dir string }
	}
	if json.Unmarshal(data, &mf) == nil {
		for _, m := range mf.Modules {
			l.manifest[m.Key] = filepath.Join(root, m.Dir)
		}
	}
	return l
}

func (l *loader) load(dir string) (*Module, error) {
	dir = filepath.Clean(dir)
	if m, ok := l.cache[dir]; ok {
		return m, nil
	}
	m, err := parseModule(dir)
	if err != nil {
		return nil, err
	}
	l.cache[dir] = m
	return m, nil
}

// resolve finds the directory of a module call. key is the dotted call path.
func (l *loader) resolve(parentDir string, call *ModuleCall, key []string) (string, error) {
	if strings.HasPrefix(call.Source, "./") || strings.HasPrefix(call.Source, "../") {
		return filepath.Join(parentDir, call.Source), nil
	}
	if dir, ok := l.manifest[strings.Join(key, ".")]; ok {
		return dir, nil
	}
	return "", fmt.Errorf("module %s: source %q is not local and not installed (run terraform init to read it)", strings.Join(key, "."), call.Source)
}
