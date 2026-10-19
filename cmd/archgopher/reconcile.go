package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/O6lvl4/archgopher/api"
	"github.com/O6lvl4/archgopher/cloud"
	"github.com/O6lvl4/archgopher/reconcile"
)

// cmdReconcile reads a declaration and puts it next to the account's bill.
func cmdReconcile(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("reconcile", flag.ContinueOnError)
	billPath := fs.String("bill", "", "the bill as Cost Explorer JSON: aws ce get-cost-and-usage --granularity MONTHLY --metrics UnblendedCost --group-by Type=DIMENSION,Key=SERVICE ...")
	asJSON := fs.Bool("json", false, "write JSON")
	if err := fs.Parse(reorder(args)); err != nil {
		return err
	}
	if fs.NArg() != 1 || *billPath == "" {
		return fmt.Errorf("reconcile takes one declaration file and --bill <costs.json>")
	}
	spec, err := readSpec(fs.Arg(0))
	if err != nil {
		return err
	}
	data, err := os.ReadFile(*billPath)
	if err != nil {
		return err
	}
	bill, err := reconcile.ParseCostExplorer(data)
	if err != nil {
		return err
	}
	res, err := api.Scout(spec)
	if err != nil {
		return err
	}
	books, err := cloud.Books()
	if err != nil {
		return err
	}
	rep := reconcile.Compare(spec.Name, res, books, cloud.BillingNames(), bill)
	if *asJSON {
		return reconcile.JSON(out, rep)
	}
	return reconcile.Markdown(out, rep)
}
