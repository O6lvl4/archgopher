package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/O6lvl4/archgopher/cloud"
	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/terraform/eval"
	"github.com/O6lvl4/archgopher/terraform/infer"
	"github.com/O6lvl4/archgopher/terraform/merge"
)

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

func cmdTerraform(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("tf", flag.ContinueOnError)
	mergePath := fs.String("merge", "", "fold the result into this declaration, keeping its assumptions, load and edges")
	output := fs.String("o", "", "write to this file instead of stdout")
	region := fs.String("region", "", "region when the aws provider does not set one")
	name := fs.String("name", "", "declaration name (default: directory name)")
	var varFiles, vars multi
	fs.Var(&varFiles, "var-file", "extra .tfvars file (repeatable)")
	fs.Var(&vars, "var", "variable as name=value (repeatable)")
	if err := fs.Parse(reorder(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("tf takes one Terraform directory")
	}
	opt, err := evalOptions(varFiles, vars)
	if err != nil {
		return err
	}
	var existing *model.Spec
	if *mergePath != "" {
		spec, err := readSpec(*mergePath)
		if err != nil {
			return err
		}
		existing = &spec
	}
	spec, warnings, err := fromTerraform(fs.Arg(0), opt, *name, *region, existing)
	if err != nil {
		return err
	}
	data, err := model.MarshalSpec(spec)
	if err != nil {
		return err
	}
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	if *output != "" {
		return os.WriteFile(*output, data, 0o644)
	}
	_, err = out.Write(data)
	return err
}

func evalOptions(varFiles, vars []string) (eval.Options, error) {
	opt := eval.Options{Vars: map[string]string{}}
	for _, f := range varFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			return opt, err
		}
		opt.VarFiles = append(opt.VarFiles, eval.File{Name: f, Data: data})
	}
	for _, v := range vars {
		k, val, ok := strings.Cut(v, "=")
		if !ok {
			return opt, fmt.Errorf("-var %q: want name=value", v)
		}
		opt.Vars[k] = val
	}
	return opt, nil
}

// fromTerraform builds a declaration from a Terraform directory, folded into
// an existing declaration when one is given.
func fromTerraform(dir string, opt eval.Options, name, region string, existing *model.Spec) (model.Spec, []string, error) {
	ev, err := eval.Evaluate(dir, opt)
	if err != nil {
		return model.Spec{}, nil, err
	}
	if name == "" {
		name = infer.DefaultName(dir)
	}
	rules := cloud.TerraformRules()
	spec, warnings := infer.Build(ev, rules, name)
	if region != "" {
		spec.Region = region
	}
	if existing != nil {
		var w []string
		spec, w = merge.Merge(*existing, spec)
		warnings = append(warnings, w...)
	}
	if cloud.FillRegion(&spec) {
		warnings = append(warnings, "no region found; using "+cloud.DefaultRegion+" (set --region)")
	}
	fmt.Fprintln(os.Stderr, infer.Cover(ev, rules).Summary())
	return spec, warnings, nil
}

func readSpec(path string) (model.Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return model.Spec{}, err
	}
	return model.ParseSpec(data)
}
