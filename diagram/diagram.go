// Package diagram draws a declaration as an architecture diagram: every node
// as its provider's icon, edges as labelled arrows, groups (a VPC) as frames,
// and, when the nodes belong to one cloud, that cloud and its region around
// them. Nodes marked attach in the catalog (log groups, alarms, an API's
// stage, a bucket's lifecycle rules) are not drawn; they are counted under the
// node that owns them, and edges through them reach the owner instead.
// Positions saved by the web UI are used when every drawn node has one;
// otherwise the nodes are laid out left to right, callers on the left.
package diagram

import (
	"fmt"
	"sort"
	"strings"

	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/scouter"
)

// Lookup gives the catalog entry of a node type; ok is false for unknown types.
type Lookup func(typ string) (meta scouter.Meta, ok bool)

// node is a drawn node.
type node struct {
	id     string
	meta   scouter.Meta
	src    model.Node
	lines  []string       // the label, the id, then the attached counts
	attach map[string]int // label → count of nodes folded into this one
	ext    bool           // a source outside the cloud, drawn left of its frame
	cx, cy float64
	degree int // edges touching it in the declaration
}

// edge is a drawn edge, between two drawn nodes.
type edge struct {
	from, to *node
	label    string
	via      []point // bends through the columns between the two, when they are apart
}

type point struct{ x, y float64 }

// frame is a drawn group.
type frame struct {
	g       model.Group
	members []*node
	x, y    float64
	w, h    float64
}

// graph is what the diagram draws.
type graph struct {
	spec     model.Spec
	nodes    []*node
	byID     map[string]*node
	edges    []*edge
	frames   []*frame
	provider string // the one cloud every inside node belongs to, or ""
	w, h     float64
	cloud    box // the cloud frame, empty when provider is ""
	region   box
}

type box struct{ x, y, w, h float64 }

// SVG draws the declaration as a self-contained SVG.
func SVG(spec model.Spec, lookup Lookup) ([]byte, error) {
	g, err := build(spec, lookup)
	if err != nil {
		return nil, err
	}
	g.layout()
	return g.svg(), nil
}

// build folds attached nodes into their owners, redirects the edges that
// touched them, and keeps one arrow per pair of drawn nodes.
func build(spec model.Spec, lookup Lookup) (*graph, error) {
	g := &graph{spec: spec, byID: map[string]*node{}}
	all := map[string]*node{}
	var order []*node
	for _, n := range spec.Nodes {
		if n.ID == "" {
			return nil, fmt.Errorf("a node has no id")
		}
		meta, ok := lookup(n.Type)
		if !ok {
			meta = scouter.Meta{Type: n.Type, Label: n.Type}
		}
		d := &node{id: n.ID, meta: meta, src: n, attach: map[string]int{}}
		all[n.ID] = d
		order = append(order, d)
	}
	out := map[string]map[string]bool{}
	near := map[string]map[string]bool{}
	add := func(m map[string]map[string]bool, a, b string) {
		if m[a] == nil {
			m[a] = map[string]bool{}
		}
		m[a][b] = true
	}
	for _, e := range spec.Edges {
		if all[e.From] == nil || all[e.To] == nil || e.From == e.To {
			continue
		}
		add(out, e.From, e.To)
		add(near, e.From, e.To)
		add(near, e.To, e.From)
		all[e.From].degree++
		all[e.To].degree++
	}
	// An attached node belongs to the one node it points at, else the one
	// node it is connected to, else the busiest node of its module; a node
	// with no owner is drawn on its own.
	owner := map[*node]*node{}
	var resolve func(d *node, seen map[*node]bool) *node
	resolve = func(d *node, seen map[*node]bool) *node {
		if !d.meta.Attach {
			return d
		}
		if o, done := owner[d]; done {
			return o
		}
		if seen[d] {
			return nil
		}
		seen[d] = true
		var cand *node
		if len(out[d.id]) == 1 {
			for id := range out[d.id] {
				cand = all[id]
			}
		} else if len(near[d.id]) == 1 {
			for id := range near[d.id] {
				cand = all[id]
			}
		} else {
			cand = busiest(order, module(d.id), d)
		}
		var o *node
		if cand != nil {
			o = resolve(cand, seen)
		}
		owner[d] = o
		return o
	}
	for _, d := range order {
		resolve(d, map[*node]bool{})
	}
	rep := map[string]*node{} // every node → the drawn node standing for it
	for _, d := range order {
		o := d
		if d.meta.Attach {
			o = owner[d]
		}
		if o == nil {
			o = d // no owner: drawn on its own
		}
		rep[d.id] = o
		if o == d {
			g.nodes = append(g.nodes, d)
			g.byID[d.id] = d
		} else {
			o.attach[d.meta.Label]++
		}
	}
	// One arrow per pair of drawn nodes; several edges between them (a read
	// and a write) share it and list their labels.
	between := map[[2]*node]*edge{}
	for _, e := range spec.Edges {
		a, b := rep[e.From], rep[e.To]
		if a == nil || b == nil || a == b {
			continue
		}
		label := edgeLabel(e)
		if d := between[[2]*node{a, b}]; d != nil {
			if label != "" && !strings.Contains(" · "+d.label+" · ", " · "+label+" · ") {
				d.label = strings.TrimPrefix(d.label+" · "+label, " · ")
			}
			continue
		}
		d := &edge{from: a, to: b, label: label}
		between[[2]*node{a, b}] = d
		g.edges = append(g.edges, d)
	}
	indeg := map[*node]int{}
	for _, e := range g.edges {
		indeg[e.to]++
	}
	for _, d := range g.nodes {
		d.ext = (d.meta.Type == scouter.EntryType || d.meta.External) && indeg[d] == 0
		d.lines = labelLines(d)
	}
	g.provider = oneProvider(g.nodes)
	for _, gr := range spec.Groups {
		f := &frame{g: gr}
		for _, d := range g.nodes {
			if d.src.Group == gr.ID {
				f.members = append(f.members, d)
			}
		}
		g.frames = append(g.frames, f)
	}
	return g, nil
}

// busiest is the node of a module with the most edges that is not attached
// itself, other than skip.
func busiest(nodes []*node, mod string, skip *node) *node {
	var best *node
	for _, d := range nodes {
		if d == skip || d.meta.Attach || module(d.id) != mod {
			continue
		}
		if best == nil || d.degree > best.degree {
			best = d
		}
	}
	return best
}

// module is the id before its first dot; a plain id is its own module.
func module(id string) string {
	if i := strings.IndexByte(id, '.'); i >= 0 {
		return id[:i]
	}
	return id
}

// edgeLabel is the kind, with the calls per unit when they are not one.
func edgeLabel(e model.Edge) string {
	label := e.Kind
	if e.PerUnit != nil && *e.PerUnit != 1 {
		per := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.4f", *e.PerUnit), "0"), ".")
		label = strings.TrimSpace(label + " ×" + per)
	}
	return label
}

// labelLines are the lines under a node's icon: its catalog label (with the
// instance count), its id, then the attached nodes as "label ×n" chips.
func labelLines(d *node) []string {
	title := d.meta.Label
	if d.src.Instances > 1 {
		title += fmt.Sprintf(" ×%d", d.src.Instances)
	}
	lines := []string{title}
	if d.id != title {
		lines = append(lines, d.id)
	}
	if len(d.attach) > 0 {
		labels := make([]string, 0, len(d.attach))
		for l := range d.attach {
			labels = append(labels, l)
		}
		sort.Strings(labels)
		var chips []string
		for _, l := range labels {
			chips = append(chips, fmt.Sprintf("%s ×%d", shortLabel(l), d.attach[l]))
		}
		lines = append(lines, wrap(chips, 30)...)
	}
	return lines
}

// shortLabel drops the product name from a catalog label ("CloudWatch log
// group" → "log group") so chips stay short; single words stay as they are.
func shortLabel(label string) string {
	words := strings.Fields(label)
	if len(words) >= 2 && strings.ToUpper(words[0][:1]) == words[0][:1] && strings.ToLower(words[1]) == words[1] {
		return strings.Join(words[1:], " ")
	}
	return label
}

// wrap joins chips with " · " into lines of at most width characters.
func wrap(chips []string, width int) []string {
	var lines []string
	cur := ""
	for _, c := range chips {
		switch {
		case cur == "":
			cur = c
		case len([]rune(cur))+3+len([]rune(c)) <= width:
			cur += " · " + c
		default:
			lines = append(lines, cur)
			cur = c
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// oneProvider is the provider every inside node shares, or "" when they mix.
func oneProvider(nodes []*node) string {
	p := ""
	for _, d := range nodes {
		if d.ext || d.meta.Provider == "" {
			continue
		}
		if p == "" {
			p = d.meta.Provider
		} else if p != d.meta.Provider {
			return ""
		}
	}
	return p
}
