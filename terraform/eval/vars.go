package eval

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/O6lvl4/archgopher/terraform/config"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// rootVars gives the root module's variables their values in Terraform's
// order of precedence: defaults, terraform.tfvars, *.auto.tfvars, the files
// passed in, then -var values.
func rootVars(fsys fs.FS, m *config.Module, dir string, opt Options) (map[string]cty.Value, error) {
	vars := defaultVars(m)
	p := hclparse.NewParser()
	for _, file := range append(autoVarFiles(fsys, dir), opt.VarFiles...) {
		if err := applyVarFile(p, file, vars); err != nil {
			return nil, err
		}
	}
	for name, raw := range opt.Vars {
		vars[name] = cliValue(raw)
	}
	return vars, nil
}

// defaultVars holds each variable's default, or unknown without one.
func defaultVars(m *config.Module) map[string]cty.Value {
	vars := map[string]cty.Value{}
	for name, def := range m.Variables {
		vars[name] = cty.DynamicVal
		if def == nil {
			continue
		}
		if v, diags := def.Value(nil); !diags.HasErrors() {
			vars[name] = v
		}
	}
	return vars
}

// autoVarFiles reads the variable files Terraform loads on its own, in the
// order it applies them.
func autoVarFiles(fsys fs.FS, dir string) []File {
	names := []string{path.Join(dir, "terraform.tfvars"), path.Join(dir, "terraform.tfvars.json")}
	auto, _ := fs.Glob(fsys, path.Join(dir, "*.auto.tfvars"))
	autoJSON, _ := fs.Glob(fsys, path.Join(dir, "*.auto.tfvars.json"))
	sort.Strings(auto)
	sort.Strings(autoJSON)
	var files []File
	for _, name := range append(append(names, auto...), autoJSON...) {
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			continue
		}
		files = append(files, File{Name: name, Data: data})
	}
	return files
}

// applyVarFile sets the variables a .tfvars or .tfvars.json file assigns.
// Values that need evaluation are left as they were.
func applyVarFile(p *hclparse.Parser, file File, vars map[string]cty.Value) error {
	var f *hcl.File
	var diags hcl.Diagnostics
	if strings.HasSuffix(file.Name, ".json") {
		f, diags = p.ParseJSON(file.Data, file.Name)
	} else {
		f, diags = p.ParseHCL(file.Data, file.Name)
	}
	if diags.HasErrors() {
		return fmt.Errorf("%s", diags.Error())
	}
	attrs, diags := f.Body.JustAttributes()
	if diags.HasErrors() {
		return fmt.Errorf("%s", diags.Error())
	}
	for name, a := range attrs {
		if v, diags := a.Expr.Value(nil); !diags.HasErrors() {
			vars[name] = v
		}
	}
	return nil
}

// cliValue reads a -var value as an expression, or as a plain string when
// it is not one.
func cliValue(raw string) cty.Value {
	expr, diags := hclsyntax.ParseExpression([]byte(raw), "-var", hcl.InitialPos)
	if diags.HasErrors() {
		return cty.StringVal(raw)
	}
	if parsed, d := expr.Value(nil); !d.HasErrors() {
		return parsed
	}
	return cty.StringVal(raw)
}
