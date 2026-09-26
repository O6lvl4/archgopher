// Package eval evaluates Terraform statically: variables, locals, module
// inputs and outputs, count and for_each, and the built-in functions. Values
// that exist only after apply stay unknown. Alongside values it traces which
// resources every attribute references, which is what edges are made of.
package eval

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/O6lvl4/archgopher/terraform/config"
	"github.com/hashicorp/hcl/v2"
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
	// Instances is the count or for_each size; -1 when unknown.
	Instances int
	// Body and Scope let provider packages read the block further (IAM policies).
	Body  *hclsyntax.Body
	Scope *Scope
}

// Options tunes evaluation.
type Options struct {
	// VarFiles are extra .tfvars (or .tfvars.json) files, applied in order after the automatic ones.
	VarFiles []File
	Vars     map[string]string
}

// File is a named file content.
type File struct {
	Name string
	Data []byte
}

// Evaluated is everything read from a root module.
type Evaluated struct {
	Resources []*Resource
	// Removed are resources whose count or for_each is 0, including every
	// resource of a module call that is off.
	Removed []*Resource
	// Providers holds the evaluated attributes of each unaliased provider block.
	Providers map[string]map[string]any
	Warnings  []string
}

// Evaluate reads the root module in an OS directory and every module it calls.
func Evaluate(dir string, opt Options) (*Evaluated, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	vol := filepath.VolumeName(abs)
	rel := strings.TrimPrefix(filepath.ToSlash(abs[len(vol):]), "/")
	if rel == "" {
		rel = "."
	}
	return EvaluateFS(os.DirFS(vol+string(filepath.Separator)), rel, opt)
}

// EvaluateFS reads the root module at dir inside fsys (slash-separated) and
// every module it calls. Local module sources must stay inside fsys.
func EvaluateFS(fsys fs.FS, dir string, opt Options) (*Evaluated, error) {
	ld := config.NewLoader(fsys, dir)
	root, err := ld.Load(dir)
	if err != nil {
		return nil, err
	}
	ev := &evaluator{ld: ld, rootDir: dir, funcs: functions(), memo: map[string][]string{}, visiting: map[string]bool{}}
	vars, err := rootVars(fsys, root, dir, opt)
	if err != nil {
		return nil, err
	}
	in := &instance{mod: root, vars: vars}
	ev.evalInstance(in)
	out := &Evaluated{}
	ev.collect(in, out)
	out.Providers = ev.providers(in)
	out.Warnings = ev.warnings
	sort.Slice(out.Resources, func(i, j int) bool { return out.Resources[i].Address < out.Resources[j].Address })
	return out, nil
}

type instance struct {
	mod      *config.Module
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

// callKey is the module path of a call made from in.
func (in *instance) callKey(call *config.ModuleCall) []string {
	return append(append([]string(nil), in.path...), call.Name)
}

type evaluator struct {
	ld       *config.Loader
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
