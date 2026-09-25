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
	"regexp"
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
	files, err := filepath.Glob(*pattern)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no price books match %s (run from the repository root)", *pattern)
	}
	s := syncer{today: time.Now().UTC().Format("2006-01-02"), regions: *regions, add: split(*add)}
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
	fmt.Fprintf(out, "\n%d changed, %d not in the price lists, %d could not be resolved\n", s.changed, s.absent, s.failed)
	if s.skipped > 0 {
		fmt.Fprintf(out, "%d rows skipped: their price list needs credentials this environment does not have\n", s.skipped)
	}
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
	sources                 sources
	today, regions          string
	add                     []string
	changed, absent, failed int
	skipped                 int
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
		if len(e.Rows) > 0 {
			if err := s.table(w, id, &e, prices); err != nil {
				return err
			}
			prices[id] = e
			continue
		}
		spec, err := s.sources.of(e.Sync)
		if err != nil {
			return fmt.Errorf("%s: sync: %w", id, err)
		}
		targets := pick(e, s.regions)
		if _, any := e.Values[book.AnyRegion]; any && spec.global() {
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

// table verifies every row of a table: "{row}" in its sync spec becomes the
// row key, quoted for the price list's regular expressions. Rows are checked
// in the regions they already have, or the one asked for; the table is
// verified when every value was resolved, and dated when one changed.
func (s *syncer) table(w io.Writer, id string, e *book.Entry, all book.Book) error {
	keys := make([]string, 0, len(e.Rows))
	for k := range e.Rows {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	// Region by region, so each offer is read once for every row.
	regionSet := map[string]bool{}
	for _, row := range e.Rows {
		for r := range row {
			if s.regions == "" || strings.Contains(","+s.regions+",", ","+r+",") {
				regionSet[r] = true
			}
		}
	}
	for _, r := range s.add {
		regionSet[r] = true
	}
	regions := make([]string, 0, len(regionSet))
	for r := range regionSet {
		regions = append(regions, r)
	}
	sort.Strings(regions)
	compose := e.Compose
	if e.ComposeOf != "" {
		compose = all[e.ComposeOf].Compose
	}
	specs := map[string]priceSource{}
	for _, key := range keys {
		spec, err := s.rowSource(e.Sync, key, compose)
		if err != nil {
			return fmt.Errorf("%s.%s: sync: %w", id, key, err)
		}
		specs[key] = spec
	}
	resolved, changed := true, false
	for _, region := range regions {
		for _, key := range keys {
			row := e.Rows[key]
			old, had := row[region]
			if !had && !slices.Contains(s.add, region) {
				continue
			}
			entry := book.Entry{Unit: e.Unit, Per: e.Per, Values: map[string]book.Value{}}
			if had {
				entry.Values[region] = book.Value{Value: old, Verified: e.Verified}
			}
			v, ok := s.value(w, id+"."+key, region, entry, specs[key])
			if !ok {
				resolved = false
				continue
			}
			if (v.Value == nil) != (old == nil) || (v.Value != nil && !close(*v.Value, *old)) {
				changed = true
			}
			row[region] = v.Value
		}
	}
	if resolved {
		e.Verified = true
	}
	if changed || e.CheckedAt == "" {
		e.CheckedAt = s.today
	}
	return nil
}

var tierRow = regexp.MustCompile(`"tier"\s*:\s*"\{row\}"`)

// rowSource is the price source of one table row: the sync spec with {row}
// filled in, or, for a composed row, the sum of its parts, each resolved with
// {part} filled in.
func (s *syncer) rowSource(sync json.RawMessage, key string, compose map[string]book.Composition) (priceSource, error) {
	// In "tier" the row key is a number where the tier starts, not a pattern.
	raw := tierRow.ReplaceAllLiteralString(string(sync), `"tier": `+strconv.Quote(key))
	raw = strings.ReplaceAll(raw, "{row}", jsonQuoteMeta(key))
	c, composed := compose[key]
	if !composed {
		return s.sources.of(json.RawMessage(raw))
	}
	out := composite{in: map[string]bool{}}
	for _, r := range c.In {
		out.in[r] = true
	}
	for _, p := range c.Parts {
		spec, err := partSpec(raw, p, p.With)
		if err != nil {
			return nil, fmt.Errorf("part %s: %w", p.Name, err)
		}
		src, err := s.sources.of(spec)
		if err != nil {
			return nil, err
		}
		w := weighted{src: src, times: p.Times, name: p.Name, in: map[string]priceSource{}}
		for region, with := range p.WithIn {
			spec, err := partSpec(string(spec), book.Part{Name: p.Name}, with)
			if err != nil {
				return nil, fmt.Errorf("part %s in %s: %w", p.Name, region, err)
			}
			if w.in[region], err = s.sources.of(spec); err != nil {
				return nil, err
			}
		}
		out.parts = append(out.parts, w)
	}
	return out, nil
}

// partSpec is a row's sync spec for one part: {part} filled with the part's
// name, quoted, or its pattern, and the part's own keys laid over the spec.
func partSpec(raw string, p book.Part, overlay json.RawMessage) (json.RawMessage, error) {
	fill := jsonQuoteMeta(p.Name)
	if p.Match != "" {
		b, err := json.Marshal(p.Match)
		if err != nil {
			return nil, err
		}
		fill = string(b[1 : len(b)-1])
	}
	spec := json.RawMessage(strings.ReplaceAll(raw, "{part}", fill))
	if len(overlay) == 0 {
		return spec, nil
	}
	var base, with map[string]json.RawMessage
	if err := json.Unmarshal(spec, &base); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(overlay, &with); err != nil {
		return nil, err
	}
	for k, v := range with {
		base[k] = v
	}
	return json.Marshal(base)
}

// errNotOffered is a composed row outside the regions it is offered in.
var errNotOffered = errors.New("not offered in this region")

// composite prices a row as the sum of its parts.
type composite struct {
	in    map[string]bool
	parts []weighted
}

type weighted struct {
	src   priceSource
	times float64
	name  string
	// in holds the part's own source in a region that needs one.
	in map[string]priceSource
}

func (c composite) quote(region string) (quote, error) {
	if len(c.in) > 0 && !c.in[region] {
		return quote{}, errNotOffered
	}
	var q quote
	for _, p := range c.parts {
		src := p.src
		if s, ok := p.in[region]; ok {
			src = s
		}
		pq, err := src.quote(region)
		if err != nil {
			return quote{}, fmt.Errorf("%s: %w", p.name, err)
		}
		q.usdPerUnit += pq.usdPerUnit * p.times
	}
	q.label = fmt.Sprintf("%d parts", len(c.parts))
	return q, nil
}

func (c composite) global() bool { return false }

// jsonQuoteMeta quotes a row key for a regular expression inside a JSON string.
func jsonQuoteMeta(key string) string {
	b, _ := json.Marshal(regexp.QuoteMeta(key))
	return string(b[1 : len(b)-1])
}

// value resolves one row. It reports false when the row should stay as it is
// (or stay missing): the Price List could not say. A price that is not in the
// Price List is a row with no value, so "not offered here" is recorded too.
func (s *syncer) value(w io.Writer, id, region string, e book.Entry, spec priceSource) (book.Value, bool) {
	old, had := e.Values[region]
	q, err := spec.quote(region)
	if unauthorized(err) {
		// A price list that needs credentials the environment does not have
		// is skipped, not failed: its rows stay as they are.
		s.skipped++
		return old, false
	}
	if absent(err) && (!had || old.Value == nil) {
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
	v := round(q.usdPerUnit * perOf(e))
	result := "same"
	if old.Value == nil || !close(*old.Value, v) {
		result = "changed"
		s.changed++
	}
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s (%s)\n", id, region, show(old.Value), show(&v), result, q.label)
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
