package infer

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/O6lvl4/archgopher/model"
)

// splitVariants replaces a counted node whose instances its scouter reads
// differently (for_each over plans of different sizes) with one node per
// configuration, each carrying its own instances. Instances that differ
// only in what the scouter does not read (names, tags) stay one node. Edges
// out of the node leave every part; edges into it are shared out by
// instances, so the load the parts receive adds up to what the node did.
func (b *builder) splitVariants(nodes []model.Node, edges []model.Edge) ([]model.Node, []model.Edge) {
	var out []model.Node
	for _, n := range nodes {
		parts := b.parts(n)
		if len(parts) < 2 {
			out = append(out, n)
			continue
		}
		out = append(out, parts...)
		edges = rewire(edges, n.ID, parts)
	}
	return out, edges
}

// parts are the nodes of n's variants, alike ones merged; one part means
// n stays as it is.
func (b *builder) parts(n model.Node) []model.Node {
	r, ok := b.byAddr[n.Address]
	if !ok || len(r.Variants) < 2 {
		return nil
	}
	var parts []model.Node
	bySig := map[string]int{}
	for _, v := range r.Variants {
		one := *r
		one.Address, one.Attrs, one.Instances, one.Variants = v.Address, v.Attrs, v.Instances, nil
		p := b.nodeAs(b.partID(n.ID, v.Key), &one)
		p.Group, p.Note = n.Group, n.Note
		sig := signature(p)
		if i, ok := bySig[sig]; ok {
			parts[i].Instances = max(1, parts[i].Instances) + max(1, p.Instances)
			continue
		}
		bySig[sig] = len(parts)
		parts = append(parts, p)
	}
	return parts
}

// signature is what a part is read by: its type and the attributes its
// scouter reads.
func signature(n model.Node) string {
	s, _ := json.Marshal(struct {
		T string
		A map[string]any
	}{n.Type, n.Attributes})
	return string(s)
}

var unsafeID = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// partID names a part after its node and its for_each key or count index.
func (b *builder) partID(base, key string) string {
	stem := base + "-" + strings.Trim(unsafeID.ReplaceAllString(key, "-"), "-")
	id := stem
	for i := 2; b.used[id]; i++ {
		id = fmt.Sprintf("%s-%d", stem, i)
	}
	b.used[id] = true
	return id
}

// rewire points the edges of node id at its parts: out of every part as
// they are, into every part with the share of its instances.
func rewire(edges []model.Edge, id string, parts []model.Node) []model.Edge {
	total := 0
	for _, p := range parts {
		total += max(1, p.Instances)
	}
	var out []model.Edge
	for _, e := range edges {
		if e.From != id && e.To != id {
			out = append(out, e)
			continue
		}
		for _, p := range parts {
			pe := e
			if e.From == id {
				pe.From = p.ID
			}
			if e.To == id {
				pe = shared(pe, float64(max(1, p.Instances))/float64(total))
				pe.To = p.ID
			}
			out = append(out, pe)
		}
	}
	return out
}

// shared is e carrying share of what it did.
func shared(e model.Edge, share float64) model.Edge {
	e.PerUnit = scaled(e.PerUnit, share)
	if len(e.Ops) > 0 {
		ops := make([]model.Op, len(e.Ops))
		for i, o := range e.Ops {
			o.PerUnit = scaled(o.PerUnit, share)
			ops[i] = o
		}
		e.Ops = ops
	}
	return e
}

func scaled(p *float64, share float64) *float64 {
	v := share
	if p != nil {
		v *= *p
	}
	return &v
}
