package main

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"text/tabwriter"

	"github.com/O6lvl4/archgopher/provider/aws/pricelist"
	"github.com/O6lvl4/archgopher/provider/aws/servicequotas"
	"github.com/O6lvl4/archgopher/provider/azure/retailprices"
	"github.com/O6lvl4/archgopher/provider/gcp/billingcatalog"
)

// cmdExplore lists the prices that match filters, to write a sync spec:
//
//	explore <AWS service code> <region> [attr=regex...]
//	explore azure <Azure service name> <region> [attr=regex...]
//	explore gcp <Billing Catalog service id> <region> [attr=regex...]
//	explore servicequotas <service code> <region> [name=regex]
func cmdExplore(args []string, out io.Writer) error {
	cloud := ""
	if len(args) > 0 && (args[0] == retailprices.Source || args[0] == billingcatalog.Source || args[0] == servicequotas.Source) {
		cloud, args = args[0], args[1:]
	}
	if len(args) < 2 {
		return fmt.Errorf("explore takes [azure|gcp|servicequotas] a service, a region and optional attr=regex filters")
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
	var err error
	switch cloud {
	case retailprices.Source:
		err = exploreAzure(w, args[0], args[1], filters)
	case billingcatalog.Source:
		err = exploreGCP(w, args[0], args[1], filters)
	case servicequotas.Source:
		err = exploreQuotas(w, args[0], args[1], filters["name"])
	default:
		err = exploreAWS(w, args[0], args[1], filters)
	}
	if err != nil {
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

func exploreGCP(w io.Writer, service, region string, filters map[string]string) error {
	skus, err := billingcatalog.NewClient().Skus(service)
	if err != nil {
		return err
	}
	ms, err := billingcatalog.Find(skus, region, filters)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "DESCRIPTION\tFAMILY\tGROUP\tUNIT\tTIERS (start:usd)\tREGIONS")
	for _, m := range ms {
		var tiers []string
		for _, r := range m.Rates() {
			tiers = append(tiers, fmt.Sprintf("%g:%g", r.Start, r.UnitPrice.USD()))
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", m.Description, m.Category.ResourceFamily, m.Category.ResourceGroup, m.Unit(), strings.Join(tiers, " "), trim(strings.Join(m.ServiceRegions, ","), 40))
	}
	return nil
}

func exploreQuotas(w io.Writer, service, region, name string) error {
	quotas, err := servicequotas.NewClient().Defaults(service, region)
	if err != nil {
		return err
	}
	re, err := regexp.Compile("(?i)" + name)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "CODE\tVALUE\tUNIT\tADJUSTABLE\tGLOBAL\tNAME")
	for _, q := range quotas {
		if re.MatchString(q.QuotaName) {
			fmt.Fprintf(w, "%s\t%g\t%s\t%t\t%t\t%s\n", q.QuotaCode, q.Value, q.Unit, q.Adjustable, q.GlobalQuota, q.QuotaName)
		}
	}
	return nil
}
