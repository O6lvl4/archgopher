package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/O6lvl4/archgopher/book"
)

func cmdSync(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	pattern := fs.String("books", "catalog/*/*/books/prices.json", "price books to update in place (glob)")
	regions := fs.String("regions", "", "comma-separated regions to verify (default: every region in the book)")
	add := fs.String("add-regions", "", "comma-separated regions to add to every price that varies by region")
	check := fs.Bool("check", false, "report differences without writing")
	if err := fs.Parse(reorder(args)); err != nil {
		return err
	}
	files, err := priceBooks(*pattern)
	if err != nil {
		return err
	}
	s := syncer{today: time.Now().UTC().Format("2006-01-02"), regions: *regions, add: split(*add)}
	if err := s.files(out, files, *check); err != nil {
		return err
	}
	if err := byHand(out, files, s.add); err != nil {
		return err
	}
	if *check && (s.changed > 0 || s.failed > 0) {
		return fmt.Errorf("the price books are out of date")
	}
	return nil
}

// priceBooks is the price books the glob names; none is an error, since the
// glob is relative to the repository root.
func priceBooks(pattern string) ([]string, error) {
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no price books match %s (run from the repository root)", pattern)
	}
	return files, nil
}

type syncer struct {
	sources                 sources
	today, regions          string
	add                     []string
	changed, absent, failed int
	skipped                 int
}

// files verifies every price book, one table of rows for all of them, and
// then counts what happened.
func (s *syncer) files(out io.Writer, files []string, check bool) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tREGION\tBOOK\tPRICE LIST\tRESULT")
	for _, f := range files {
		if err := s.file(w, f, check); err != nil {
			return err
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Fprintf(out, "\n%d changed, %d not in the price lists, %d could not be resolved\n", s.changed, s.absent, s.failed)
	if s.skipped > 0 {
		fmt.Fprintf(out, "%d rows skipped: their price list needs credentials this environment does not have\n", s.skipped)
	}
	return nil
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
	for _, id := range slices.Sorted(maps.Keys(prices)) {
		e := prices[id]
		if len(e.Sync) == 0 {
			continue
		}
		if err := s.entry(w, id, &e, prices); err != nil {
			return err
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

// entry verifies one synced price, a table or a value per region.
func (s *syncer) entry(w io.Writer, id string, e *book.Entry, all book.Book) error {
	if len(e.Rows) > 0 {
		return s.table(w, id, e, all)
	}
	return s.regional(w, id, *e)
}

// regional verifies a price with one value per region: the regions it has
// (and "*" when the price list's price is the same everywhere), then the
// regions being added, unless the price is the same everywhere.
func (s *syncer) regional(w io.Writer, id string, e book.Entry) error {
	spec, err := s.sources.of(e.Sync)
	if err != nil {
		return fmt.Errorf("%s: sync: %w", id, err)
	}
	targets := pick(e, s.regions)
	if _, any := e.Values[book.AnyRegion]; any && spec.global() {
		targets = append(targets, book.AnyRegion)
	}
	for _, region := range targets {
		s.resolve(w, id, region, e, spec)
	}
	if _, any := e.Values[book.AnyRegion]; any {
		return nil
	}
	for _, region := range s.add {
		if _, has := e.Values[region]; !has {
			s.resolve(w, id, region, e, spec)
		}
	}
	return nil
}

// resolve records the value of one region when the price list could say it.
func (s *syncer) resolve(w io.Writer, id, region string, e book.Entry, spec priceSource) {
	if v, ok := s.value(w, id, region, e, spec); ok {
		e.Values[region] = v
	}
}

// value resolves one row. It reports false when the row should stay as it is
// (or stay missing): the Price List could not say. A price that is not in the
// Price List is a row with no value, so "not offered here" is recorded too.
func (s *syncer) value(w io.Writer, id, region string, e book.Entry, spec priceSource) (book.Value, bool) {
	old, had := e.Values[region]
	q, err := spec.quote(region)
	switch {
	case unauthorized(err):
		// A price list that needs credentials the environment does not have
		// is skipped, not failed: its rows stay as they are.
		s.skipped++
		return old, false
	case absent(err) && (!had || old.Value == nil):
		return s.notOffered(w, id, region, old, err), true
	case err != nil:
		s.failed++
		fmt.Fprintf(w, "%s\t%s\t%s\t-\t%v\n", id, region, show(old.Value), err)
		return old, false
	}
	return s.priced(w, id, region, e, q), true
}

// notOffered records a row the Price List does not have and nobody priced.
// A row already verified as such keeps the day it was checked.
func (s *syncer) notOffered(w io.Writer, id, region string, old book.Value, err error) book.Value {
	s.absent++
	fmt.Fprintf(w, "%s\t%s\t%s\t-\tnot offered (%v)\n", id, region, show(old.Value), err)
	if old.Verified {
		return old
	}
	return book.Value{Verified: true, CheckedAt: s.today}
}

// priced records the Price List's price for a row, in the entry's unit.
// A verified row whose value did not change keeps the day it was checked.
func (s *syncer) priced(w io.Writer, id, region string, e book.Entry, q quote) book.Value {
	old := e.Values[region]
	v := round(q.usdPerUnit * perOf(e))
	result := "same"
	if old.Value == nil || !close(*old.Value, v) {
		result = "changed"
		s.changed++
	}
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s (%s)\n", id, region, show(old.Value), show(&v), result, q.label)
	if result == "same" && old.Verified {
		return old
	}
	return book.Value{Value: &v, Verified: true, CheckedAt: s.today}
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

// listed reports whether a region is in a comma-separated list; an empty
// list names every region.
func listed(list, region string) bool {
	return list == "" || strings.Contains(","+list+",", ","+region+",")
}

func pick(e book.Entry, only string) []string {
	var out []string
	for r := range e.Values {
		if r != book.AnyRegion && listed(only, r) {
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
