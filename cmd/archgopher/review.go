package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/O6lvl4/archgopher/api"
	"github.com/O6lvl4/archgopher/review"
)

func cmdReview(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("review", flag.ContinueOnError)
	region := fs.String("region", "", "region when the aws provider does not set one")
	asJSON := fs.Bool("json", false, "write JSON")
	fail := fs.Bool("fail", false, "exit non-zero when anything departs")
	var varFiles, vars multi
	fs.Var(&varFiles, "var-file", "extra .tfvars file (repeatable)")
	fs.Var(&vars, "var", "variable as name=value (repeatable)")
	if err := fs.Parse(reorder(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("review takes one declaration or Terraform directory")
	}
	spec, err := declarationAt(fs.Arg(0), nil, *region, varFiles, vars)
	if err != nil {
		return err
	}
	found, err := api.Review(spec)
	if err != nil {
		return err
	}
	if *asJSON {
		if found == nil {
			found = []review.Finding{}
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		err = enc.Encode(found)
	} else {
		_, err = io.WriteString(out, review.Text(found))
	}
	if err == nil && *fail && len(found) > 0 {
		return fmt.Errorf("%d departure(s) from the known practices", len(found))
	}
	return err
}
