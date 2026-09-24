package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/O6lvl4/arch-scouter/aws"
	"github.com/O6lvl4/arch-scouter/scout"
	"github.com/O6lvl4/arch-scouter/terraform"
)

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

func cmdTerraform(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("tf", flag.ContinueOnError)
	merge := fs.String("merge", "", "fold the result into this declaration, keeping its assumptions, load and edges")
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
	opt := terraform.Options{Vars: map[string]string{}}
	for _, f := range varFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		opt.VarFiles = append(opt.VarFiles, terraform.File{Name: f, Data: data})
	}
	for _, v := range vars {
		k, val, ok := strings.Cut(v, "=")
		if !ok {
			return fmt.Errorf("-var %q: want name=value", v)
		}
		opt.Vars[k] = val
	}
	ev, err := terraform.Evaluate(dir, opt)
	if err != nil {
		return err
	}
	if *region != "" {
		ev.Region = *region
	}
	n := *name
	if n == "" {
		n = terraform.DefaultName(dir)
	}
	spec, warnings := terraform.Build(ev, aws.TerraformRules(), n)
	if *merge != "" {
		data, err := os.ReadFile(*merge)
		if err != nil {
			return err
		}
		existing, err := scout.ParseSpec(data)
		if err != nil {
			return err
		}
		var w []string
		spec, w = terraform.Merge(existing, spec)
		warnings = append(warnings, w...)
	}
	if spec.Region == "" {
		spec.Region = "us-east-1"
		warnings = append(warnings, "no region found; using us-east-1 (set --region)")
	}
	data, err := scout.MarshalSpec(spec)
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
