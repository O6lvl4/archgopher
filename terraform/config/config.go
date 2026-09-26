// Package config reads Terraform source into module structures: variables,
// locals, resources, module calls and outputs, as unevaluated expressions.
// It resolves module sources to directories inside an fs.FS and does nothing else.
package config

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
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

func parseModule(fsys fs.FS, dir string) (*Module, error) {
	files, err := fs.Glob(fsys, path.Join(dir, "*.tf"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no .tf files in %s", dir)
	}
	sort.Strings(files)
	m := &Module{Dir: dir, Variables: map[string]hcl.Expression{}, Locals: map[string]hcl.Expression{}, Outputs: map[string]hcl.Expression{}}
	p := hclparse.NewParser()
	for _, name := range files {
		src, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		f, diags := p.ParseHCL(src, name)
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

// blockReaders maps a top-level block type to the reader that records it.
var blockReaders = map[string]func(*Module, *hclsyntax.Block){
	"variable": (*Module).addVariable,
	"locals":   (*Module).addLocals,
	"resource": (*Module).addResource,
	"data":     (*Module).addResource,
	"module":   (*Module).addCall,
	"output":   (*Module).addOutput,
	"provider": (*Module).addProvider,
}

// addBlock records a top-level block; types Terraform has but the model does
// not read (terraform, moved, import and the like) are skipped.
func (m *Module) addBlock(b *hclsyntax.Block) {
	if read, ok := blockReaders[b.Type]; ok {
		read(m, b)
	}
}

func (m *Module) addVariable(b *hclsyntax.Block) {
	if len(b.Labels) == 1 {
		m.Variables[b.Labels[0]] = attrExpr(b.Body, "default")
	}
}

func (m *Module) addLocals(b *hclsyntax.Block) {
	for name, a := range b.Body.Attributes {
		m.Locals[name] = a.Expr
	}
}

func (m *Module) addResource(b *hclsyntax.Block) {
	if len(b.Labels) != 2 {
		return
	}
	mode := "managed"
	if b.Type == "data" {
		mode = "data"
	}
	m.Resources = append(m.Resources, &ResourceBlock{
		Mode: mode, Type: b.Labels[0], Name: b.Labels[1], Body: b.Body,
		Count: attrExpr(b.Body, "count"), ForEach: attrExpr(b.Body, "for_each"),
	})
}

func (m *Module) addCall(b *hclsyntax.Block) {
	if len(b.Labels) != 1 {
		return
	}
	m.Calls = append(m.Calls, &ModuleCall{
		Name: b.Labels[0], Source: literalString(attrExpr(b.Body, "source")), Body: b.Body,
		Count: attrExpr(b.Body, "count"), ForEach: attrExpr(b.Body, "for_each"),
	})
}

func (m *Module) addOutput(b *hclsyntax.Block) {
	if len(b.Labels) != 1 {
		return
	}
	if v := attrExpr(b.Body, "value"); v != nil {
		m.Outputs[b.Labels[0]] = v
	}
}

func (m *Module) addProvider(b *hclsyntax.Block) {
	m.Providers = append(m.Providers, b)
}

// literalString returns the value of an expression that is a string without
// references, or "" for anything else.
func literalString(expr hcl.Expression) string {
	if expr == nil {
		return ""
	}
	v, diags := expr.Value(nil)
	if diags.HasErrors() || v.Type().FriendlyName() != "string" {
		return ""
	}
	return v.AsString()
}

func attrExpr(body *hclsyntax.Body, name string) hcl.Expression {
	if a, ok := body.Attributes[name]; ok {
		return a.Expr
	}
	return nil
}

// Loader resolves module sources and caches parsed modules. Paths are
// slash-separated and relative to the root of fsys.
type Loader struct {
	FS       fs.FS
	root     string
	cache    map[string]*Module
	manifest map[string]string // module key ("a.b") -> directory, from terraform init
}

// NewLoader reads the module manifest that terraform init leaves, if any.
func NewLoader(fsys fs.FS, root string) *Loader {
	l := &Loader{FS: fsys, root: root, cache: map[string]*Module{}, manifest: map[string]string{}}
	data, err := fs.ReadFile(fsys, path.Join(root, ".terraform", "modules", "modules.json"))
	if err != nil {
		return l
	}
	var mf struct {
		Modules []struct{ Key, Dir string }
	}
	if json.Unmarshal(data, &mf) == nil {
		for _, m := range mf.Modules {
			l.manifest[m.Key] = path.Join(root, m.Dir)
		}
	}
	return l
}

// Load parses the module in dir, once.
func (l *Loader) Load(dir string) (*Module, error) {
	dir = path.Clean(dir)
	if m, ok := l.cache[dir]; ok {
		return m, nil
	}
	m, err := parseModule(l.FS, dir)
	if err != nil {
		return nil, err
	}
	l.cache[dir] = m
	return m, nil
}

// Resolve finds the directory of a module call. key is the call path from the root.
func (l *Loader) Resolve(parentDir string, call *ModuleCall, key []string) (string, error) {
	if strings.HasPrefix(call.Source, "./") || strings.HasPrefix(call.Source, "../") {
		dir := path.Join(parentDir, call.Source)
		if dir == ".." || strings.HasPrefix(dir, "../") {
			return "", fmt.Errorf("module %s: source %q is outside the files given", strings.Join(key, "."), call.Source)
		}
		return dir, nil
	}
	if dir, ok := l.manifest[strings.Join(key, ".")]; ok {
		return dir, nil
	}
	return "", fmt.Errorf("module %s: source %q is not local and not installed (run terraform init to read it)", strings.Join(key, "."), call.Source)
}

// Has reports whether the module declares a resource or data block.
func (m *Module) Has(mode, typ, name string) bool {
	for _, r := range m.Resources {
		if r.Mode == mode && r.Type == typ && r.Name == name {
			return true
		}
	}
	return false
}

// Attr returns the expression of an attribute, or nil.
func Attr(body *hclsyntax.Body, name string) hcl.Expression {
	return attrExpr(body, name)
}
