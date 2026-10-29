package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/O6lvl4/archgopher/api"
	"github.com/O6lvl4/archgopher/flow"
)

func cmdFlow(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("flow", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "write JSON")
	var varFiles, vars multi
	fs.Var(&varFiles, "var-file", "extra .tfvars file (repeatable)")
	fs.Var(&vars, "var", "variable as name=value (repeatable)")
	if err := fs.Parse(reorder(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("flow takes one declaration or Terraform directory")
	}
	spec, _, err := reviewInput(fs.Arg(0), varFiles, vars)
	if err != nil {
		return err
	}
	rs, err := api.Flow(spec)
	if err != nil {
		return err
	}
	if *asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(rs)
	}
	_, err = io.WriteString(out, flow.Text(rs))
	return err
}
