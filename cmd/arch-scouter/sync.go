package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/O6lvl4/arch-scouter/aws/pricelist"
	"github.com/O6lvl4/arch-scouter/scout"
)

func cmdSync(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	book := fs.String("book", "aws/books/prices.json", "price book to update in place")
	regions := fs.String("regions", "", "comma-separated regions (default: every region in the book)")
	check := fs.Bool("check", false, "report differences without writing")
	if err := fs.Parse(reorder(args)); err != nil {
		return err
	}
	data, err := os.ReadFile(*book)
	if err != nil {
		return err
	}
	var prices scout.Book
	if err := json.Unmarshal(data, &prices); err != nil {
		return err
	}
	client := pricelist.NewClient()
	today := time.Now().UTC().Format("2006-01-02")
	ids := make([]string, 0, len(prices))
	for id := range prices {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tREGION\tBOOK\tPRICE LIST\tRESULT")
	changed, failed := 0, 0
	for _, id := range ids {
		e := prices[id]
		if len(e.Sync) == 0 {
			continue
		}
		var spec pricelist.Spec
		if err := json.Unmarshal(e.Sync, &spec); err != nil {
			return fmt.Errorf("%s: sync: %w", id, err)
		}
		for _, region := range pick(e, *regions) {
			old := e.Values[region]
			m, err := client.Resolve(spec, region)
			if err != nil {
				failed++
				fmt.Fprintf(w, "%s\t%s\t%s\t-\t%v\n", id, region, show(old.Value), err)
				continue
			}
			v := round(m.USD * perOf(e))
			result := "same"
			if old.Value == nil || !close(*old.Value, v) {
				result, changed = "changed", changed+1
			}
			if result == "changed" || !old.Verified {
				e.Values[region] = scout.Value{Value: &v, Verified: true, CheckedAt: today}
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s (%s, %s)\n", id, region, show(old.Value), show(&v), result, m.Product.Attributes["usagetype"], m.Dimension.Unit)
		}
		prices[id] = e
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Fprintf(out, "\n%d changed, %d could not be resolved\n", changed, failed)
	if *check {
		if changed > 0 || failed > 0 {
			return fmt.Errorf("the price book is out of date")
		}
		return nil
	}
	data, err = scout.MarshalBook(prices)
	if err != nil {
		return err
	}
	return os.WriteFile(*book, data, 0o644)
}

func cmdExplore(args []string, out io.Writer) error {
	if len(args) < 2 {
		return fmt.Errorf("explore takes a service, a region and optional attr=regex filters")
	}
	filters := map[string]string{}
	for _, f := range args[2:] {
		k, v, ok := strings.Cut(f, "=")
		if !ok {
			return fmt.Errorf("filter %q: want attr=regex", f)
		}
		filters[k] = v
	}
	o, err := pricelist.NewClient().Offer(args[0], args[1])
	if err != nil {
		return err
	}
	ms, err := o.Find(filters)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "USAGETYPE\tFAMILY\tOPERATION\tGROUP\tUNIT\tBEGIN\tUSD\tDESCRIPTION")
	for _, m := range ms {
		a := m.Product.Attributes
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%g\t%s\n", a["usagetype"], m.Product.ProductFamily, a["operation"], a["group"], m.Dimension.Unit, m.Dimension.BeginRange, m.USD, trim(m.Dimension.Description, 70))
	}
	return w.Flush()
}

func pick(e scout.Entry, only string) []string {
	var out []string
	for r := range e.Values {
		if r == scout.AnyRegion {
			continue
		}
		if only == "" || strings.Contains(","+only+",", ","+r+",") {
			out = append(out, r)
		}
	}
	sort.Strings(out)
	return out
}

func perOf(e scout.Entry) float64 {
	if e.Per == 0 {
		return 1
	}
	return e.Per
}

func close(a, b float64) bool { return math.Abs(a-b) <= 1e-9*math.Max(math.Abs(a), math.Abs(b)) }

func show(v *float64) string {
	if v == nil {
		return "unknown"
	}
	return fmt.Sprintf("%g", *v)
}

func trim(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// round keeps 12 significant digits so 0.2 does not become 0.19999999999999998.
func round(v float64) float64 {
	r, _ := strconv.ParseFloat(strconv.FormatFloat(v, 'g', 12, 64), 64)
	return r
}
