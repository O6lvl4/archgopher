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
	dir := fs.Arg(0)
	opt := eval.Options{Vars: map[string]string{}}
	for _, f := range varFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		opt.VarFiles = append(opt.VarFiles, eval.File{Name: f, Data: data})
	}
	for _, v := range vars {
		k, val, ok := strings.Cut(v, "=")
		if !ok {
			return fmt.Errorf("-var %q: want name=value", v)
		}
		opt.Vars[k] = val
	}
	ev, err := eval.Evaluate(dir, opt)
	if err != nil {
		return err
	}
	n := *name
	if n == "" {
		n = infer.DefaultName(dir)
	}
	spec, warnings := infer.Build(ev, cloud.TerraformRules(), n)
	if *region != "" {
		spec.Region = *region
	}
	if *mergePath != "" {
		data, err := os.ReadFile(*mergePath)
		if err != nil {
			return err
		}
		existing, err := model.ParseSpec(data)
		if err != nil {
			return err
		}
		var w []string
		spec, w = merge.Merge(existing, spec)
		warnings = append(warnings, w...)
	}
	if cloud.FillRegion(&spec) {
		warnings = append(warnings, "no region found; using "+cloud.DefaultRegion+" (set --region)")
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
