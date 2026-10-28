package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/O6lvl4/archgopher/api"
	"github.com/O6lvl4/archgopher/cloud"
	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/review"
	"github.com/O6lvl4/archgopher/terraform/eval"
	"github.com/O6lvl4/archgopher/terraform/infer"
)

func cmdReview(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("review", flag.ContinueOnError)
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
	spec, ev, err := reviewInput(fs.Arg(0), varFiles, vars)
	if err != nil {
		return err
	}
	found, err := api.Review(spec, ev)
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

// reviewInput reads a declaration file, or a Terraform directory with the
// resources it evaluates to, so the rules can read settings as well as the graph.
func reviewInput(path string, varFiles, vars []string) (model.Spec, *eval.Evaluated, error) {
	st, err := os.Stat(path)
	if err != nil {
		return model.Spec{}, nil, err
	}
	if !st.IsDir() {
		spec, err := readSpec(path)
		return spec, nil, err
	}
	opt, err := evalOptions(varFiles, vars)
	if err != nil {
		return model.Spec{}, nil, err
	}
	ev, err := eval.Evaluate(path, opt)
	if err != nil {
		return model.Spec{}, nil, err
	}
	spec, _ := infer.Build(ev, cloud.TerraformRules(), infer.DefaultName(path))
	return spec, ev, nil
}
