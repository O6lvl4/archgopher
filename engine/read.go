package engine

import (
	"sort"

	"github.com/O6lvl4/archgopher/book"
	"github.com/O6lvl4/archgopher/meter"
	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/scouter"
)

// reader lets scouters read nodes and collects the reference values they used
// that nobody has checked or that are unknown.
type reader struct {
	reg        scouter.Registry
	books      book.Books
	region     string
	unverified map[meter.RefUse]bool
}

func newReader(reg scouter.Registry, books book.Books, region string) *reader {
	return &reader{reg: reg, books: books, region: region, unverified: map[meter.RefUse]bool{}}
}

// read has the node's scouter read demand d; a node without one is skipped.
func (rd *reader) read(n model.Node, d model.Demand) NodeResult {
	nr := NodeResult{ID: n.ID, Type: n.Type, Label: n.Type, Address: n.Address, Note: n.Note, Stale: n.Stale, Demand: d}
	s, ok := rd.reg[n.Type]
	if !ok {
		nr.Skipped = "no scouter for " + n.Type
		return nr
	}
	m := s.Meta()
	nr.Label = m.Label
	r := meter.NewRecorder(rd.region, rd.books)
	if m.SLA != "" {
		nr.SLA = rd.sla(m.SLA)
	}
	nr.Latency = latencyOf(n)
	if err := s.Scout(n, d, r); err != nil {
		nr.Error = err.Error()
	}
	nr.Costs, nr.Limits = r.Costs(), r.Limits()
	for _, u := range r.Refs() {
		rd.note(u)
	}
	return nr
}

// sla looks up an SLA by id; nil if the book has no entry for the region.
func (rd *reader) sla(id string) *Availability {
	e, v, err := rd.books.SLAs.Lookup(id, rd.region)
	if err != nil {
		return nil
	}
	rd.note(meter.RefUse{Book: book.SLAs, ID: id, Region: rd.region, Verified: v.Verified, Known: v.Value != nil, Source: e.Source})
	return &Availability{ID: id, Value: v.Value}
}

// note remembers a reference value that is unchecked or unknown.
func (rd *reader) note(u meter.RefUse) {
	if !u.Verified || !u.Known {
		rd.unverified[u] = true
	}
}

func latencyOf(n model.Node) *Latency {
	p50, ok50 := n.Assumptions[scouter.LatencyP50]
	p99, ok99 := n.Assumptions[scouter.LatencyP99]
	if !ok50 && !ok99 {
		return nil
	}
	l := &Latency{}
	if v, ok := number(p50); ok {
		l.P50Ms = v
	}
	if v, ok := number(p99); ok {
		l.P99Ms = v
	} else {
		l.P99Ms = l.P50Ms
	}
	return l
}

func number(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	}
	return 0, false
}

func sortedRefs(set map[meter.RefUse]bool) []meter.RefUse {
	out := make([]meter.RefUse, 0, len(set))
	for u := range set {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Book != out[j].Book {
			return out[i].Book < out[j].Book
		}
		return out[i].ID < out[j].ID
	})
	return out
}
