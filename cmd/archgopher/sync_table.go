package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/O6lvl4/archgopher/book"
)

// table verifies every row of a table: "{row}" in its sync spec becomes the
// row key, quoted for the price list's regular expressions. Rows are checked
// in the regions they already have, or the one asked for; the table is
// verified when every value was resolved, and dated when one changed.
func (s *syncer) table(w io.Writer, id string, e *book.Entry, all book.Book) error {
	specs, err := s.rowSources(id, e, all)
	if err != nil {
		return err
	}
	keys := slices.Sorted(maps.Keys(e.Rows))
	resolved, changed := true, false
	// Region by region, so each offer is read once for every row.
	for _, region := range s.tableRegions(e) {
		for _, key := range keys {
			ok, diff := s.cell(w, id, key, region, e, specs[key])
			resolved = resolved && ok
			changed = changed || diff
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

// tableRegions is the regions a table is checked in: those its rows have
// that were asked for, and those being added.
func (s *syncer) tableRegions(e *book.Entry) []string {
	set := map[string]bool{}
	for _, row := range e.Rows {
		for r := range row {
			if listed(s.regions, r) {
				set[r] = true
			}
		}
	}
	for _, r := range s.add {
		set[r] = true
	}
	return slices.Sorted(maps.Keys(set))
}

// rowSources is the price source of every row of a table, by row key.
// A table may borrow its compositions from another table of the same book.
func (s *syncer) rowSources(id string, e *book.Entry, all book.Book) (map[string]priceSource, error) {
	compose := e.Compose
	if e.ComposeOf != "" {
		compose = all[e.ComposeOf].Compose
	}
	specs := make(map[string]priceSource, len(e.Rows))
	for _, key := range slices.Sorted(maps.Keys(e.Rows)) {
		spec, err := s.rowSource(e.Sync, key, compose)
		if err != nil {
			return nil, fmt.Errorf("%s.%s: sync: %w", id, key, err)
		}
		specs[key] = spec
	}
	return specs, nil
}

// cell verifies one row of a table in one region, in place. A row without
// the region is left alone unless the region is being added. It reports
// whether the price list could say, and whether the value changed.
func (s *syncer) cell(w io.Writer, id, key, region string, e *book.Entry, spec priceSource) (resolved, changed bool) {
	row := e.Rows[key]
	old, had := row[region]
	if !had && !slices.Contains(s.add, region) {
		return true, false
	}
	entry := book.Entry{Unit: e.Unit, Per: e.Per, Values: map[string]book.Value{}}
	if had {
		entry.Values[region] = book.Value{Value: old, Verified: e.Verified}
	}
	v, ok := s.value(w, id+"."+key, region, entry, spec)
	if !ok {
		return false, false
	}
	row[region] = v.Value
	return true, !same(v.Value, old)
}

// same reports whether two optional prices are equal, a missing one equal
// only to another missing one.
func same(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return close(*a, *b)
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
		part, err := s.partSource(raw, p)
		if err != nil {
			return nil, err
		}
		out.parts = append(out.parts, part)
	}
	return out, nil
}

// partSource is the price source of one part of a composed row, with the
// sources it needs in the regions where the part is sold differently.
func (s *syncer) partSource(raw string, p book.Part) (weighted, error) {
	spec, err := partSpec(raw, p, p.With)
	if err != nil {
		return weighted{}, fmt.Errorf("part %s: %w", p.Name, err)
	}
	src, err := s.sources.of(spec)
	if err != nil {
		return weighted{}, err
	}
	w := weighted{src: src, times: p.Times, name: p.Name, in: map[string]priceSource{}}
	for region, with := range p.WithIn {
		spec, err := partSpec(string(spec), book.Part{Name: p.Name}, with)
		if err != nil {
			return weighted{}, fmt.Errorf("part %s in %s: %w", p.Name, region, err)
		}
		if w.in[region], err = s.sources.of(spec); err != nil {
			return weighted{}, err
		}
	}
	return w, nil
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
	maps.Copy(base, with)
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
