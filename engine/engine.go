// Package engine computes: it validates a declaration, pushes load from the
// entries through the graph in topological order, lets each node's scouter
// read it, and composes paths. It knows no provider; scouters and books are
// handed to it.
package engine

import (
	"fmt"
	"strings"

	"github.com/O6lvl4/archgopher/book"
	"github.com/O6lvl4/archgopher/meter"
	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/scouter"
	"github.com/O6lvl4/archgopher/traffic"
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
	// Pools are the prices billed on the whole account's usage, with the
	// lines that share each.
	Pools    []meter.Pool `json:"pools,omitempty"`
	Warnings []string     `json:"warnings"`
}

// NodeResult is one node's readings.
type NodeResult struct {
	ID      string       `json:"id"`
	Type    string       `json:"type"`
	Label   string       `json:"label"`
	Address string       `json:"address,omitempty"`
	Note    string       `json:"note,omitempty"`
	Stale   bool         `json:"stale,omitempty"`
	Demand  model.Demand `json:"demand"`
	// Load is the load the node brings in, and LoadBasis how it was worked
	// out when it was given as traffic.
	Load       *model.Load   `json:"load,omitempty"`
	LoadBasis  string        `json:"loadBasis,omitempty"`
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

// GroupKind is the demand a group's scouter reads: GB a month moved between
// its nodes.
const GroupKind = "transfer"

// Run validates the spec, propagates load and reads every node.
// Structural errors (duplicate IDs, dangling edges, cycles, bad kinds) fail the run;
// a node's own errors are reported on the node and load still flows through it.
func Run(spec model.Spec, reg scouter.Registry, books book.Books) (Result, error) {
	spec, arrivals, err := resolveTraffic(spec)
	if err != nil {
		return Result{}, err
	}
	g, err := buildGraph(spec, reg)
	if err != nil {
		return Result{}, err
	}
	demand := g.propagate()
	rd := newReader(reg, books, spec.Region)
	res := Result{Name: spec.Name, Region: spec.Region}
	res.Nodes = g.readNodes(rd, demand, arrivals)
	res.Groups = g.readGroups(spec.Groups, rd, demand)
	all := results(res.Nodes, res.Groups)
	res.Pools = sharePools(all, books.Prices, spec.Region)
	res.MonthlyUSD, res.UnpricedCosts = sumCosts(all)
	res.Unverified = sortedRefs(rd.unverified)
	res.Paths, res.Warnings = g.paths(res.Nodes)
	return res, nil
}

type arrival struct{ basis, err string }

// resolveTraffic turns each node's traffic into its load. Traffic that cannot
// be read is an error on that node, which then brings no load.
func resolveTraffic(spec model.Spec) (model.Spec, map[string]arrival, error) {
	out := map[string]arrival{}
	nodes := make([]model.Node, len(spec.Nodes))
	for i, n := range spec.Nodes {
		nodes[i] = n
		if n.Traffic == nil {
			continue
		}
		if n.Load != nil {
			return spec, nil, fmt.Errorf("node %q: give load or traffic, not both", n.ID)
		}
		r, err := traffic.Resolve(*n.Traffic)
		if err != nil {
			out[n.ID] = arrival{err: "traffic: " + err.Error()}
			continue
		}
		l := r.Load
		nodes[i].Load = &l
		out[n.ID] = arrival{basis: r.Basis}
	}
	spec.Nodes = nodes
	return spec, out, nil
}

// readNodes reads every node in topological order, with the load it brings
// in and any error its traffic had.
func (g *graph) readNodes(rd *reader, demand map[string]model.Demand, arrivals map[string]arrival) []NodeResult {
	var out []NodeResult
	for _, id := range g.order {
		n := g.nodes[id]
		nr := rd.read(n, demand[id])
		nr.Load, nr.LoadBasis = n.Load, arrivals[id].basis
		if e := arrivals[id].err; e != "" {
			nr.Error = joinError(nr.Error, e)
		}
		out = append(out, nr)
	}
	return out
}

// readGroups reads the groups that have a scouter and traffic between their nodes.
func (g *graph) readGroups(groups []model.Group, rd *reader, demand map[string]model.Demand) []NodeResult {
	var out []NodeResult
	for _, gr := range groups {
		gb := g.crossing(gr.ID, demand)
		if gr.Type == "" || gb == 0 {
			continue
		}
		n := model.Node{ID: gr.ID, Type: gr.Type, Assumptions: gr.Assumptions}
		out = append(out, rd.read(n, model.Demand{GroupKind: {Monthly: gb}}))
	}
	return out
}

// results points at every node and group result, so pools and totals can
// update them in place.
func results(nodes, groups []NodeResult) []*NodeResult {
	var all []*NodeResult
	for i := range nodes {
		all = append(all, &nodes[i])
	}
	for i := range groups {
		all = append(all, &groups[i])
	}
	return all
}

// sumCosts sets each result's monthly total and returns the grand total with
// the number of lines that have no price.
func sumCosts(all []*NodeResult) (float64, int) {
	var total float64
	var unpriced int
	for _, nr := range all {
		nr.MonthlyUSD = 0
		for _, c := range nr.Costs {
			if c.MonthlyUSD == nil {
				unpriced++
				continue
			}
			nr.MonthlyUSD += *c.MonthlyUSD
		}
		total += nr.MonthlyUSD
	}
	return total, unpriced
}

// sharePools bills every pool of the declaration once, over all its nodes'
// lines. A pool that cannot be billed puts its error on each node sharing it.
func sharePools(nodes []*NodeResult, prices book.Book, region string) []meter.Pool {
	var lines []meter.Owned
	byID := map[string]*NodeResult{}
	for _, n := range nodes {
		byID[n.ID] = n
		for i := range n.Costs {
			lines = append(lines, meter.Owned{Node: n.ID, Cost: &n.Costs[i]})
		}
	}
	pools := meter.Share(lines, prices, region)
	for _, p := range pools {
		if p.Error != "" {
			blame(p, byID)
		}
	}
	return pools
}

// blame puts a pool's error once on each node sharing it.
func blame(p meter.Pool, byID map[string]*NodeResult) {
	seen := map[string]bool{}
	for _, m := range p.Members {
		if n := byID[m.Node]; !seen[m.Node] && !strings.Contains(n.Error, p.Error) {
			seen[m.Node] = true
			n.Error = joinError(n.Error, p.Error)
		}
	}
}

// joinError appends msg to an error text that may be empty.
func joinError(err, msg string) string {
	return strings.TrimPrefix(err+"; "+msg, "; ")
}
