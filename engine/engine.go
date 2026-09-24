// Package engine computes: it validates a declaration, pushes load from the
// entries through the graph in topological order, lets each node's scouter
// read it, and composes paths. It knows no provider; scouters and books are
// handed to it.
package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/O6lvl4/archgopher/book"
	"github.com/O6lvl4/archgopher/meter"
	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/scouter"
)

// Result is everything the engine read from a spec.
type Result struct {
	Name   string       `json:"name"`
	Region string       `json:"region"`
	Nodes  []NodeResult `json:"nodes"`
	Paths  []PathResult `json:"paths"`
	// Groups are the readings of groups with traffic between their nodes.
	Groups     []NodeResult `json:"groups,omitempty"`
	MonthlyUSD float64      `json:"monthlyUsd"`
	// UnpricedCosts counts cost lines whose price is unknown; MonthlyUSD excludes them.
	UnpricedCosts int `json:"unpricedCosts"`
	// Unverified lists reference values that nobody has checked, or that are unknown.
	Unverified []meter.RefUse `json:"unverified"`
	Warnings   []string       `json:"warnings"`
}

// NodeResult is one node's readings.
type NodeResult struct {
	ID         string        `json:"id"`
	Type       string        `json:"type"`
	Label      string        `json:"label"`
	Address    string        `json:"address,omitempty"`
	Note       string        `json:"note,omitempty"`
	Stale      bool          `json:"stale,omitempty"`
	Demand     model.Demand  `json:"demand"`
	Costs      []meter.Cost  `json:"costs"`
	Limits     []meter.Limit `json:"limits"`
	Latency    *Latency      `json:"latency,omitempty"`
	SLA        *Availability `json:"sla,omitempty"`
	MonthlyUSD float64       `json:"monthlyUsd"`
	// Members is set on a rolled-up result (a pattern): the ids it sums. Its
	// costs are already counted in the members, so totals skip it.
	Members []string `json:"members,omitempty"`
	// Skipped says why no scouter read the node; Error says why the scouter failed.
	Skipped string `json:"skipped,omitempty"`
	Error   string `json:"error,omitempty"`
}

// MinHeadroom is the tightest known headroom, nil if none is known.
func (n NodeResult) MinHeadroom() *float64 {
	var min *float64
	for _, l := range n.Limits {
		if l.Headroom != nil && (min == nil || *l.Headroom < *min) {
			h := *l.Headroom
			min = &h
		}
	}
	return min
}

// Latency is one round trip through a node.
type Latency struct {
	P50Ms float64 `json:"p50Ms"`
	P99Ms float64 `json:"p99Ms"`
}

// Availability is the SLA of a node; Value is nil when unknown.
type Availability struct {
	ID    string   `json:"id"`
	Value *float64 `json:"value"`
}

// PathResult composes one path from an entry to a sink.
// Latencies add (so p99 is an upper bound), availabilities multiply.
type PathResult struct {
	Nodes          []string `json:"nodes"`
	P50Ms          float64  `json:"p50Ms"`
	P99Ms          float64  `json:"p99Ms"`
	Availability   float64  `json:"availability"`
	MissingLatency []string `json:"missingLatency"`
	MissingSLA     []string `json:"missingSla"`
}

// MaxPaths caps path enumeration on dense graphs.
const MaxPaths = 200

// Run validates the spec, propagates load and reads every node.
// Structural errors (duplicate IDs, dangling edges, cycles, bad kinds) fail the run;
// a node's own errors are reported on the node and load still flows through it.
func Run(spec model.Spec, reg scouter.Registry, books book.Books) (Result, error) {
	g, err := buildGraph(spec, reg)
	if err != nil {
		return Result{}, err
	}
	res := Result{Name: spec.Name, Region: spec.Region}
	demand := g.propagate()
	unverified := map[meter.RefUse]bool{}
	for _, id := range g.order {
		n := g.nodes[id]
		nr := readNode(n, demand[id], reg, books, spec.Region, unverified)
		res.MonthlyUSD += nr.MonthlyUSD
		for _, c := range nr.Costs {
			if c.MonthlyUSD == nil {
				res.UnpricedCosts++
			}
		}
		res.Nodes = append(res.Nodes, nr)
	}
	var warnings []string
	for _, gr := range spec.Groups {
		gb := g.crossing(gr.ID, demand)
		if gr.Type == "" || gb == 0 {
			continue
		}
		n := model.Node{ID: gr.ID, Type: gr.Type, Assumptions: gr.Assumptions}
		nr := readNode(n, model.Demand{GroupKind: {Monthly: gb}}, reg, books, spec.Region, unverified)
		res.MonthlyUSD += nr.MonthlyUSD
		for _, c := range nr.Costs {
			if c.MonthlyUSD == nil {
				res.UnpricedCosts++
			}
		}
		res.Groups = append(res.Groups, nr)
	}
	for _, e := range spec.Edges {
		if e.KB != nil && (g.nodes[e.From].Group == "" || g.nodes[e.From].Group != g.nodes[e.To].Group) {
			warnings = append(warnings, fmt.Sprintf("edge %s -> %s: kb is read only between nodes of one group", e.From, e.To))
		}
	}
	res.Unverified = sortedRefs(unverified)
	var pathWarnings []string
	res.Paths, pathWarnings = g.paths(res.Nodes)
	res.Warnings = append(warnings, pathWarnings...)
	return res, nil
}

// GroupKind is the demand a group's scouter reads: GB a month moved between
// its nodes.
const GroupKind = "transfer"

// crossing is the GB a month that edges with kb move between two nodes of a group.
func (g *graph) crossing(group string, demand map[string]model.Demand) float64 {
	var gb float64
	for _, id := range g.order {
		for _, e := range g.outgoing[id] {
			if e.KB == nil || g.nodes[e.From].Group != group || g.nodes[e.To].Group != group {
				continue
			}
			gb += demand[e.From].Total().Monthly * e.Factor() * *e.KB / 1024 / 1024
		}
	}
	return gb
}

func readNode(n model.Node, d model.Demand, reg scouter.Registry, books book.Books, region string, unverified map[meter.RefUse]bool) NodeResult {
	nr := NodeResult{ID: n.ID, Type: n.Type, Label: n.Type, Address: n.Address, Note: n.Note, Stale: n.Stale, Demand: d}
	s, ok := reg[n.Type]
	if !ok {
		nr.Skipped = "no scouter for " + n.Type
		return nr
	}
	m := s.Meta()
	nr.Label = m.Label
	r := meter.NewRecorder(region, books)
	if m.SLA != "" {
		e, v, err := books.SLAs.Lookup(m.SLA, region)
		if err == nil {
			nr.SLA = &Availability{ID: m.SLA, Value: v.Value}
			use := meter.RefUse{Book: book.SLAs, ID: m.SLA, Region: region, Verified: v.Verified, Known: v.Value != nil, Source: e.Source}
			if !use.Verified || !use.Known {
				unverified[use] = true
			}
		}
	}
	nr.Latency = latencyOf(n)
	if err := s.Scout(n, d, r); err != nil {
		nr.Error = err.Error()
	}
	nr.Costs, nr.Limits = r.Costs(), r.Limits()
	for _, c := range nr.Costs {
		if c.MonthlyUSD != nil {
			nr.MonthlyUSD += *c.MonthlyUSD
		}
	}
	for _, u := range r.Refs() {
		if !u.Verified || !u.Known {
			unverified[u] = true
		}
	}
	return nr
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

type graph struct {
	nodes    map[string]model.Node
	outgoing map[string][]model.Edge
	order    []string
	kinds    map[string]string // default kind per node
}

func buildGraph(spec model.Spec, reg scouter.Registry) (*graph, error) {
	g := &graph{nodes: map[string]model.Node{}, outgoing: map[string][]model.Edge{}, kinds: map[string]string{}}
	groups := map[string]bool{}
	for _, gr := range spec.Groups {
		if gr.ID == "" {
			return nil, fmt.Errorf("a %s group has no id", gr.Kind)
		}
		if groups[gr.ID] {
			return nil, fmt.Errorf("duplicate group id %q", gr.ID)
		}
		groups[gr.ID] = true
	}
	var ids []string
	for _, n := range spec.Nodes {
		if n.ID == "" {
			return nil, fmt.Errorf("a node of type %s has no id", n.Type)
		}
		if _, dup := g.nodes[n.ID]; dup {
			return nil, fmt.Errorf("duplicate node id %q", n.ID)
		}
		if n.Group != "" && !groups[n.Group] {
			return nil, fmt.Errorf("node %q: no group %q", n.ID, n.Group)
		}
		g.nodes[n.ID] = n
		ids = append(ids, n.ID)
		g.kinds[n.ID] = "request"
		if s, ok := reg[n.Type]; ok && len(s.Meta().Kinds) > 0 {
			g.kinds[n.ID] = s.Meta().Kinds[0]
		}
		if n.Load != nil && (n.Load.Monthly < 0 || n.Load.PeakPerSecond < 0) {
			return nil, fmt.Errorf("node %q: load must not be negative", n.ID)
		}
	}
	indegree := map[string]int{}
	for _, e := range spec.Edges {
		label := fmt.Sprintf("edge %s -> %s", e.From, e.To)
		if _, ok := g.nodes[e.From]; !ok {
			return nil, fmt.Errorf("%s: no node %q", label, e.From)
		}
		to, ok := g.nodes[e.To]
		if !ok {
			return nil, fmt.Errorf("%s: no node %q", label, e.To)
		}
		if e.Factor() < 0 {
			return nil, fmt.Errorf("%s: perUnit must not be negative", label)
		}
		if e.KB != nil && *e.KB < 0 {
			return nil, fmt.Errorf("%s: kb must not be negative", label)
		}
		if s, ok := reg[to.Type]; ok && e.Kind != "" && !contains(s.Meta().Kinds, e.Kind) {
			return nil, fmt.Errorf("%s: %s accepts %s, not %q", label, to.Type, strings.Join(s.Meta().Kinds, ", "), e.Kind)
		}
		g.outgoing[e.From] = append(g.outgoing[e.From], e)
		indegree[e.To]++
	}
	queue := []string{}
	for _, id := range ids {
		if indegree[id] == 0 {
			queue = append(queue, id)
		}
	}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		g.order = append(g.order, id)
		for _, e := range g.outgoing[id] {
			indegree[e.To]--
			if indegree[e.To] == 0 {
				queue = append(queue, e.To)
			}
		}
	}
	if len(g.order) != len(ids) {
		var cyclic []string
		for _, id := range ids {
			if indegree[id] > 0 {
				cyclic = append(cyclic, id)
			}
		}
		return nil, fmt.Errorf("edges form a cycle through %s", strings.Join(cyclic, ", "))
	}
	return g, nil
}

func (g *graph) propagate() map[string]model.Demand {
	demand := map[string]model.Demand{}
	for _, id := range g.order {
		d := demand[id]
		if d == nil {
			d = model.Demand{}
			demand[id] = d
		}
		if l := g.nodes[id].Load; l != nil {
			k := g.kinds[id]
			d[k] = d[k].Add(*l)
		}
		out := d.Total()
		for _, e := range g.outgoing[id] {
			k := e.Kind
			if k == "" {
				k = g.kinds[e.To]
			}
			if demand[e.To] == nil {
				demand[e.To] = model.Demand{}
			}
			demand[e.To][k] = demand[e.To][k].Add(out.Scale(e.Factor()))
		}
	}
	return demand
}

func (g *graph) paths(results []NodeResult) ([]PathResult, []string) {
	byID := map[string]NodeResult{}
	for _, r := range results {
		byID[r.ID] = r
	}
	var out []PathResult
	var warnings []string
	var walk func(id string, trail []string)
	walk = func(id string, trail []string) {
		if len(out) >= MaxPaths {
			return
		}
		trail = append(trail, id)
		if len(g.outgoing[id]) == 0 {
			out = append(out, composePath(append([]string(nil), trail...), byID))
			return
		}
		seen := map[string]bool{}
		for _, e := range g.outgoing[id] {
			if !seen[e.To] {
				seen[e.To] = true
				walk(e.To, trail)
			}
		}
	}
	for _, id := range g.order {
		if g.nodes[id].Load != nil {
			walk(id, nil)
		}
	}
	if len(out) >= MaxPaths {
		warnings = append(warnings, fmt.Sprintf("more than %d paths; only the first %d are listed", MaxPaths, MaxPaths))
	}
	return out, warnings
}

func composePath(ids []string, byID map[string]NodeResult) PathResult {
	p := PathResult{Nodes: ids, Availability: 1, MissingLatency: []string{}, MissingSLA: []string{}}
	for i, id := range ids {
		n := byID[id]
		if i == 0 && n.Type == scouter.EntryType {
			continue // the entry is the caller, not a hop
		}
		if n.Latency != nil {
			p.P50Ms += n.Latency.P50Ms
			p.P99Ms += n.Latency.P99Ms
		} else {
			p.MissingLatency = append(p.MissingLatency, id)
		}
		if n.SLA != nil && n.SLA.Value != nil {
			p.Availability *= *n.SLA.Value
		} else {
			p.MissingSLA = append(p.MissingSLA, id)
		}
	}
	return p
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
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
