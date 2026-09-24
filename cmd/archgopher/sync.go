package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/O6lvl4/archgopher/book"
	"github.com/O6lvl4/archgopher/provider/aws/pricelist"
)

func cmdSync(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	pattern := fs.String("books", "catalog/aws/*/books/prices.json", "price books to update in place (glob)")
	regions := fs.String("regions", "", "comma-separated regions to verify (default: every region in the book)")
	add := fs.String("add-regions", "", "comma-separated regions to add to every price that varies by region")
	check := fs.Bool("check", false, "report differences without writing")
	if err := fs.Parse(reorder(args)); err != nil {
		return err
	}
	files, err := filepath.Glob(*pattern)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no price books match %s (run from the repository root)", *pattern)
	}
	s := syncer{client: pricelist.NewClient(), today: time.Now().UTC().Format("2006-01-02"), regions: *regions, add: split(*add)}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tREGION\tBOOK\tPRICE LIST\tRESULT")
	for _, f := range files {
		if err := s.file(w, f, *check); err != nil {
			return err
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Fprintf(out, "\n%d changed, %d not in the Price List, %d could not be resolved\n", s.changed, s.absent, s.failed)
	if len(s.add) > 0 {
		if err := byHand(out, files, s.add); err != nil {
			return err
		}
	}
	if *check && (s.changed > 0 || s.failed > 0) {
		return fmt.Errorf("the price books are out of date")
	}
	return nil
}

type syncer struct {
	client                  *pricelist.Client
	today, regions          string
	add                     []string
	changed, absent, failed int
}

// file verifies one price book against the Price List and rewrites it unless checking.
func (s *syncer) file(w io.Writer, path string, check bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var prices book.Book
	if err := json.Unmarshal(data, &prices); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	ids := make([]string, 0, len(prices))
	for id := range prices {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		e := prices[id]
		if len(e.Sync) == 0 {
			continue
		}
		var spec pricelist.Spec
		if err := json.Unmarshal(e.Sync, &spec); err != nil {
			return fmt.Errorf("%s: sync: %w", id, err)
		}
		targets := pick(e, s.regions)
		if _, any := e.Values[book.AnyRegion]; any && spec.OfferRegion != "" {
			targets = append(targets, book.AnyRegion)
		}
		for _, region := range targets {
			if v, ok := s.value(w, id, region, e, spec); ok {
				e.Values[region] = v
			}
		}
		if _, any := e.Values[book.AnyRegion]; !any {
			for _, region := range s.add {
				if _, has := e.Values[region]; has {
					continue
				}
				if v, ok := s.value(w, id, region, e, spec); ok {
					e.Values[region] = v
				}
			}
		}
		prices[id] = e
	}
	if check {
		return nil
	}
	out, err := book.Marshal(prices)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

// value resolves one row. It reports false when the row should stay as it is
// (or stay missing): the Price List could not say. A price that is not in the
// Price List is a row with no value, so "not offered here" is recorded too.
func (s *syncer) value(w io.Writer, id, region string, e book.Entry, spec pricelist.Spec) (book.Value, bool) {
	old, had := e.Values[region]
	m, err := s.client.Resolve(spec, region)
	if errors.Is(err, pricelist.ErrAbsent) && (!had || old.Value == nil) {
		s.absent++
		fmt.Fprintf(w, "%s\t%s\t%s\t-\tnot offered (%v)\n", id, region, show(old.Value), err)
		if had && old.Verified {
			return old, true
		}
		return book.Value{Verified: true, CheckedAt: s.today}, true
	}
	if err != nil {
		s.failed++
		fmt.Fprintf(w, "%s\t%s\t%s\t-\t%v\n", id, region, show(old.Value), err)
		return old, false
	}
	v := round(spec.PerUnit(m.USD) * perOf(e))
	result := "same"
	if old.Value == nil || !close(*old.Value, v) {
		result = "changed"
		s.changed++
	}
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s (%s, %s)\n", id, region, show(old.Value), show(&v), result, m.Product.Attributes["usagetype"], m.Dimension.Unit)
	if result == "same" && old.Verified {
		return old, true
	}
	return book.Value{Value: &v, Verified: true, CheckedAt: s.today}, true
}

// byHand lists the rows a new region still needs that the Price List cannot
// give: prices without a sync spec, and quotas and SLAs that vary by region.
func byHand(out io.Writer, priceFiles []string, regions []string) error {
	var missing []string
	for _, f := range priceFiles {
		dir := filepath.Dir(f)
		for _, name := range []string{"prices", "quotas", "slas"} {
			data, err := os.ReadFile(filepath.Join(dir, name+".json"))
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return err
			}
			var b book.Book
			if err := json.Unmarshal(data, &b); err != nil {
				return err
			}
			for id, e := range b {
				if _, any := e.Values[book.AnyRegion]; any || (name == "prices" && len(e.Sync) > 0) {
					continue
				}
				for _, r := range regions {
					if _, has := e.Values[r]; !has {
						missing = append(missing, fmt.Sprintf("%s\t%s\t%s", filepath.Join(dir, name+".json"), id, r))
					}
				}
			}
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	fmt.Fprintf(out, "\n%d rows need a value by hand:\n", len(missing))
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, m := range missing {
		fmt.Fprintln(w, m)
	}
	return w.Flush()
}

func split(list string) []string {
	var out []string
	for _, r := range strings.Split(list, ",") {
		if r = strings.TrimSpace(r); r != "" {
			out = append(out, r)
		}
	}
	return out
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

func pick(e book.Entry, only string) []string {
	var out []string
	for r := range e.Values {
		if r == book.AnyRegion {
			continue
		}
		if only == "" || strings.Contains(","+only+",", ","+r+",") {
			out = append(out, r)
		}
	}
	sort.Strings(out)
	return out
}

func perOf(e book.Entry) float64 {
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
