package main

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/O6lvl4/archgopher/provider/aws/pricelist"
	"github.com/O6lvl4/archgopher/provider/azure/retailprices"
)

// cmdExplore lists the prices that match filters, to write a sync spec:
//
//	explore <AWS service code> <region> [attr=regex...]
//	explore azure <Azure service name> <region> [attr=regex...]
func cmdExplore(args []string, out io.Writer) error {
	azure := len(args) > 0 && args[0] == retailprices.Source
	if azure {
		args = args[1:]
	}
	if len(args) < 2 {
		return fmt.Errorf("explore takes [azure] a service, a region and optional attr=regex filters")
	}
	filters := map[string]string{}
	for _, f := range args[2:] {
		k, v, ok := strings.Cut(f, "=")
		if !ok {
			return fmt.Errorf("filter %q: want attr=regex", f)
		}
		filters[k] = v
	}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if azure {
		if err := exploreAzure(w, args[0], args[1], filters); err != nil {
			return err
		}
	} else if err := exploreAWS(w, args[0], args[1], filters); err != nil {
		return err
	}
	return w.Flush()
}

func exploreAWS(w io.Writer, service, region string, filters map[string]string) error {
	o, err := pricelist.NewClient().Offer(service, region)
	if err != nil {
		return err
	}
	ms, err := o.Find(filters)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "USAGETYPE\tFAMILY\tOPERATION\tGROUP\tUNIT\tBEGIN\tUSD\tDESCRIPTION")
	for _, m := range ms {
		a := m.Product.Attributes
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%g\t%s\n", a["usagetype"], m.Product.ProductFamily, a["operation"], a["group"], m.Dimension.Unit, m.Dimension.BeginRange, m.USD, trim(m.Dimension.Description, 70))
	}
	return nil
}

func exploreAzure(w io.Writer, service, region string, filters map[string]string) error {
	items, err := retailprices.NewClient().Items(service, region)
	if err != nil {
		return err
	}
	ms, err := retailprices.Find(items, filters)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "PRODUCT\tSKU\tARMSKU\tMETER\tUNIT\tTIER\tUSD\tTYPE")
	for _, m := range ms {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%g\t%g\t%s\n", m.ProductName, m.SkuName, m.ArmSkuName, m.MeterName, m.UnitOfMeasure, m.TierMinimum, m.RetailPrice, m.Type)
	}
	return nil
}
