package engine

import (
	"fmt"
	"slices"
	"strings"

	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/scouter"
)

type graph struct {
	nodes    map[string]model.Node
	outgoing map[string][]model.Edge
	order    []string
	kinds    map[string]string // default kind per node
}

// buildGraph checks the declaration's structure and orders its nodes
// topologically.
func buildGraph(spec model.Spec, reg scouter.Registry) (*graph, error) {
	g := &graph{nodes: map[string]model.Node{}, outgoing: map[string][]model.Edge{}, kinds: map[string]string{}}
	groups, err := groupIDs(spec.Groups)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(spec.Nodes))
	for _, n := range spec.Nodes {
		if err := g.addNode(n, groups, reg); err != nil {
			return nil, err
		}
		ids = append(ids, n.ID)
	}
	indegree := map[string]int{}
	for _, e := range spec.Edges {
		if err := g.addEdge(e, reg); err != nil {
			return nil, err
		}
		indegree[e.To]++
	}
	if err := g.sort(ids, indegree); err != nil {
		return nil, err
	}
	return g, nil
}

// groupIDs is the set of group ids, which must be present and unique.
func groupIDs(groups []model.Group) (map[string]bool, error) {
	ids := map[string]bool{}
	for _, gr := range groups {
		if gr.ID == "" {
			return nil, fmt.Errorf("a %s group has no id", gr.Kind)
		}
		if ids[gr.ID] {
			return nil, fmt.Errorf("duplicate group id %q", gr.ID)
		}
		ids[gr.ID] = true
	}
	return ids, nil
}

func (g *graph) addNode(n model.Node, groups map[string]bool, reg scouter.Registry) error {
	if n.ID == "" {
		return fmt.Errorf("a node of type %s has no id", n.Type)
	}
	if _, dup := g.nodes[n.ID]; dup {
		return fmt.Errorf("duplicate node id %q", n.ID)
	}
	if n.Group != "" && !groups[n.Group] {
		return fmt.Errorf("node %q: no group %q", n.ID, n.Group)
	}
	if n.Load != nil && (n.Load.Monthly < 0 || n.Load.PeakPerSecond < 0) {
		return fmt.Errorf("node %q: load must not be negative", n.ID)
	}
	g.nodes[n.ID] = n
	g.kinds[n.ID] = defaultKind(reg, n.Type)
	return nil
}

// defaultKind is the first kind a type's scouter accepts, or "request".
func defaultKind(reg scouter.Registry, typ string) string {
	if s, ok := reg[typ]; ok && len(s.Meta().Kinds) > 0 {
		return s.Meta().Kinds[0]
	}
	return "request"
}

func (g *graph) addEdge(e model.Edge, reg scouter.Registry) error {
	label := fmt.Sprintf("edge %s -> %s", e.From, e.To)
	if _, ok := g.nodes[e.From]; !ok {
		return fmt.Errorf("%s: no node %q", label, e.From)
	}
	to, ok := g.nodes[e.To]
	if !ok {
		return fmt.Errorf("%s: no node %q", label, e.To)
	}
	if err := checkOps(e, label, reg[to.Type]); err != nil {
		return err
	}
	g.outgoing[e.From] = append(g.outgoing[e.From], e)
	return nil
}

// sort orders ids so every edge goes forward (Kahn's algorithm), and names
// the nodes left on a cycle if there is one.
func (g *graph) sort(ids []string, indegree map[string]int) error {
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
	if len(g.order) == len(ids) {
		return nil
	}
	var cyclic []string
	for _, id := range ids {
		if indegree[id] > 0 {
			cyclic = append(cyclic, id)
		}
	}
	return fmt.Errorf("edges form a cycle through %s", strings.Join(cyclic, ", "))
}

// checkOps validates an edge's operations against what the target accepts.
func checkOps(e model.Edge, label string, target scouter.Scouter) error {
	if len(e.Ops) > 0 && (e.Kind != "" || e.PerUnit != nil || e.KB != nil) {
		return fmt.Errorf("%s: give ops or kind, perUnit and kb, not both", label)
	}
	for _, op := range e.Operations() {
		if err := checkOp(op, label, target); err != nil {
			return err
		}
	}
	return nil
}

func checkOp(op model.Op, label string, target scouter.Scouter) error {
	if op.Factor() < 0 {
		return fmt.Errorf("%s: perUnit must not be negative", label)
	}
	if op.KB != nil && *op.KB < 0 {
		return fmt.Errorf("%s: kb must not be negative", label)
	}
	if target != nil && op.Kind != "" && !slices.Contains(target.Meta().Kinds, op.Kind) {
		return fmt.Errorf("%s: %s accepts %s, not %q", label, target.Meta().Type, strings.Join(target.Meta().Kinds, ", "), op.Kind)
	}
	return nil
}

// propagate pushes load from the nodes that bring it in along every edge, in
// topological order, so each node's demand is complete before it passes it on.
func (g *graph) propagate() map[string]model.Demand {
	demand := map[string]model.Demand{}
	for _, id := range g.order {
		d := g.own(id, demand)
		// A node passes on its own work; the sizes of what it received stay with it.
		out := d.Total().Plain()
		for _, e := range g.outgoing[id] {
			g.push(e, out, demand)
		}
	}
	return demand
}

// own is a node's demand: what reached it plus the load it brings in.
func (g *graph) own(id string, demand map[string]model.Demand) model.Demand {
	d := demand[id]
	if d == nil {
		d = model.Demand{}
		demand[id] = d
	}
	if l := g.nodes[id].Load; l != nil {
		k := g.kinds[id]
		d[k] = d[k].Add(*l)
	}
	return d
}

// push adds load out, scaled and sized by each operation of e, to its target.
func (g *graph) push(e model.Edge, out model.Load, demand map[string]model.Demand) {
	to := demand[e.To]
	if to == nil {
		to = model.Demand{}
		demand[e.To] = to
	}
	for _, op := range e.Operations() {
		k := op.Kind
		if k == "" {
			k = g.kinds[e.To]
		}
		l := out.Scale(op.Factor())
		if op.KB != nil {
			l = l.Sized(*op.KB)
		}
		to[k] = to[k].Add(l)
	}
}

// crossing is the GB a month that edges with kb move between two nodes of a group.
func (g *graph) crossing(group string, demand map[string]model.Demand) float64 {
	var gb float64
	for _, id := range g.order {
		for _, e := range g.outgoing[id] {
			if g.nodes[e.From].Group == group && g.nodes[e.To].Group == group {
				gb += sizedGB(e, demand[e.From].Total().Monthly)
			}
		}
	}
	return gb
}

// sizedGB is the GB an edge moves when its source does monthly units of work.
func sizedGB(e model.Edge, monthly float64) float64 {
	var gb float64
	for _, op := range e.Operations() {
		if op.KB != nil {
			gb += monthly * op.Factor() * *op.KB / 1024 / 1024
		}
	}
	return gb
}
