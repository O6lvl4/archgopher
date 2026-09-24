// Package pattern is the L3 layer: reusable architectures. A pattern takes
// parameters and expands into a fragment of nodes and edges, the way a
// Terraform module expands into resources. A declaration places a pattern as
// one node; Expand replaces it with its fragment before the engine runs, and
// Rollup adds one result per pattern that sums what it expanded into.
package pattern

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/O6lvl4/arch-scouter/engine"
	"github.com/O6lvl4/arch-scouter/field"
	"github.com/O6lvl4/arch-scouter/meter"
	"github.com/O6lvl4/arch-scouter/model"
	"github.com/O6lvl4/arch-scouter/scouter"
)

// Fragment is what a pattern expands into.
type Fragment struct {
	Nodes []model.Node
	Edges []model.Edge
	// In receives the pattern node's incoming edges and its load.
	In string
	// Out sends the pattern node's outgoing edges; empty means the pattern is a sink.
	Out string
}

// Pattern is one reusable architecture.
type Pattern interface {
	Meta() scouter.Meta
	Params() []field.Field
	Expand(params map[string]any) (Fragment, error)
}

// Def builds a Pattern from a tagged parameter struct.
type Def[P any] struct {
	Info  scouter.Meta
	Build func(p P) Fragment
}

func (d Def[P]) Meta() scouter.Meta { return d.Info }

func (d Def[P]) Params() []field.Field { return field.FieldsOf(reflect.TypeFor[P]()) }

func (d Def[P]) Expand(params map[string]any) (Fragment, error) {
	var p P
	if err := field.Decode(params, &p, "parameter"); err != nil {
		return Fragment{}, err
	}
	return d.Build(p), nil
}

// Registry maps a pattern type to its definition.
type Registry map[string]Pattern

// Register adds patterns, panicking on a duplicate type.
func (reg Registry) Register(ps ...Pattern) {
	for _, p := range ps {
		t := p.Meta().Type
		if _, dup := reg[t]; dup {
			panic("duplicate pattern " + t)
		}
		reg[t] = p
	}
}

// Types lists registered types in order.
func (reg Registry) Types() []string {
	out := make([]string, 0, len(reg))
	for t := range reg {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// Group is what one pattern node expanded into.
type Group struct {
	Members []string
	// In and Out are the member ids where load enters and leaves.
	In, Out string
}

// Expansion maps each pattern node to its group.
type Expansion map[string]Group

// Sep joins a pattern node id and an inner id: "orders/fn".
const Sep = "/"

// maxDepth bounds patterns that expand into patterns.
const maxDepth = 8

// Expand replaces every pattern node, recursively, with its fragment.
func Expand(spec model.Spec, reg Registry) (model.Spec, Expansion, error) {
	exp := Expansion{}
	for depth := 0; ; depth++ {
		next, done, err := expandOnce(spec, reg, exp)
		if err != nil {
			return spec, nil, err
		}
		if done {
			return next, exp, nil
		}
		if depth == maxDepth {
			return spec, nil, fmt.Errorf("patterns nest deeper than %d levels", maxDepth)
		}
		spec = next
	}
}

func expandOnce(spec model.Spec, reg Registry, exp Expansion) (model.Spec, bool, error) {
	out := model.Spec{Name: spec.Name, Region: spec.Region}
	ends := map[string]Fragment{}
	for _, n := range spec.Nodes {
		p, ok := reg[n.Type]
		if !ok {
			out.Nodes = append(out.Nodes, n)
			continue
		}
		f, err := p.Expand(n.Assumptions)
		if err != nil {
			return spec, false, fmt.Errorf("pattern %s: %w", n.ID, err)
		}
		ends[n.ID] = f
		exp.add(n.ID, f)
		out.Nodes = append(out.Nodes, inner(n, f)...)
		out.Edges = append(out.Edges, innerEdges(n.ID, f)...)
	}
	if len(ends) == 0 {
		return spec, true, nil
	}
	for _, e := range spec.Edges {
		rewired, err := rewire(e, ends)
		if err != nil {
			return spec, false, err
		}
		out.Edges = append(out.Edges, rewired)
	}
	return out, false, nil
}

func (exp Expansion) add(id string, f Fragment) {
	g := Group{In: id + Sep + f.In}
	if f.Out != "" {
		g.Out = id + Sep + f.Out
	}
	for _, n := range f.Nodes {
		g.Members = append(g.Members, id+Sep+n.ID)
	}
	exp[id] = g
	// A pattern inside a pattern: the outer one owns the inner one's members too.
	for outer, og := range exp {
		for _, m := range og.Members {
			if m == id {
				og.Members = append(og.Members, g.Members...)
				exp[outer] = og
				break
			}
		}
	}
}

func inner(host model.Node, f Fragment) []model.Node {
	out := make([]model.Node, 0, len(f.Nodes))
	for _, n := range f.Nodes {
		n.ID = host.ID + Sep + n.ID
		if n.ID == host.ID+Sep+f.In && host.Load != nil {
			l := *host.Load
			if n.Load != nil {
				l = l.Add(*n.Load)
			}
			n.Load = &l
		}
		out = append(out, n)
	}
	return out
}

func innerEdges(host string, f Fragment) []model.Edge {
	out := make([]model.Edge, 0, len(f.Edges))
	for _, e := range f.Edges {
		e.From, e.To = host+Sep+e.From, host+Sep+e.To
		out = append(out, e)
	}
	return out
}

func rewire(e model.Edge, ends map[string]Fragment) (model.Edge, error) {
	if f, ok := ends[e.To]; ok {
		e.To = e.To + Sep + f.In
	}
	if f, ok := ends[e.From]; ok {
		if f.Out == "" {
			return e, fmt.Errorf("pattern %s has no outlet, so it cannot send load to %s", e.From, e.To)
		}
		e.From = e.From + Sep + f.Out
	}
	return e, nil
}

// Rollup appends one result per pattern node that sums its members, and
// returns the result with them. Totals are unchanged: members are already counted.
func Rollup(res engine.Result, spec model.Spec, exp Expansion, reg Registry) engine.Result {
	byID := map[string]engine.NodeResult{}
	for _, n := range res.Nodes {
		byID[n.ID] = n
	}
	for _, n := range spec.Nodes {
		g, ok := exp[n.ID]
		if !ok {
			continue
		}
		res.Nodes = append(res.Nodes, rollup(n, reg[n.Type], g, byID))
	}
	return res
}

// rollup sums a group. Its demand is what leaves through the outlet (what
// enters, for a sink), so edges drawn from the pattern carry the right volume.
func rollup(n model.Node, p Pattern, grp Group, byID map[string]engine.NodeResult) engine.NodeResult {
	g := engine.NodeResult{ID: n.ID, Type: n.Type, Label: p.Meta().Label, Note: n.Note, Members: grp.Members, Demand: model.Demand{}}
	end := grp.Out
	if end == "" {
		end = grp.In
	}
	if m, ok := byID[end]; ok {
		g.Demand = m.Demand
	}
	var errs []error
	for _, id := range grp.Members {
		m, ok := byID[id]
		if !ok {
			continue
		}
		short := strings.TrimPrefix(id, n.ID+Sep)
		g.MonthlyUSD += m.MonthlyUSD
		for _, c := range m.Costs {
			c.Name = short + ": " + c.Name
			g.Costs = append(g.Costs, c)
		}
		for _, l := range m.Limits {
			l.Name = short + ": " + l.Name
			g.Limits = append(g.Limits, l)
		}
		if m.Error != "" {
			errs = append(errs, fmt.Errorf("%s: %s", short, m.Error))
		}
	}
	if err := errors.Join(errs...); err != nil {
		g.Error = err.Error()
	}
	if g.Costs == nil {
		g.Costs = []meter.Cost{}
	}
	return g
}
