package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/O6lvl4/archgopher/api"
	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/report"
)

func cmdDiff(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	specPath := fs.String("spec", "", "declaration that Terraform directories are folded into (its load, assumptions and edges)")
	region := fs.String("region", "", "region when the aws provider does not set one")
	asJSON := fs.Bool("json", false, "write JSON instead of Markdown")
	var varFiles, vars multi
	fs.Var(&varFiles, "var-file", "extra .tfvars file (repeatable)")
	fs.Var(&vars, "var", "variable as name=value (repeatable)")
	if err := fs.Parse(reorder(args)); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return fmt.Errorf("diff takes two declarations or Terraform directories: before and after")
	}
	var existing *model.Spec
	if *specPath != "" {
		s, err := readSpec(*specPath)
		if err != nil {
			return err
		}
		existing = &s
	}
	var specs [2]model.Spec
	for i, path := range fs.Args() {
		spec, err := declarationAt(path, existing, *region, varFiles, vars)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		specs[i] = spec
	}
	before, err := api.Scout(specs[0])
	if err != nil {
		return err
	}
	after, err := api.Scout(specs[1])
	if err != nil {
		return err
	}
	d := report.Compare(before, after)
	if *asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(d)
	}
	return report.DiffMarkdown(out, d)
}

// declarationAt reads a declaration file, or builds one from a Terraform
// directory, folded into existing when given.
func declarationAt(path string, existing *model.Spec, region string, varFiles, vars []string) (model.Spec, error) {
	st, err := os.Stat(path)
	if err != nil {
		return model.Spec{}, err
	}
	if !st.IsDir() {
		return readSpec(path)
	}
	opt, err := evalOptions(varFiles, vars)
	if err != nil {
		return model.Spec{}, err
	}
	name := ""
	if existing != nil {
		name = existing.Name
	}
	spec, warnings, err := fromTerraform(path, opt, name, region, existing)
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	return spec, err
}
