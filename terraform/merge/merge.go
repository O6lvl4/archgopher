// Package merge folds a declaration built from Terraform into one people
// have edited. The Terraform address is the seam.
package merge

import (
	"fmt"

	"github.com/O6lvl4/archgopher/model"
)

// Merge folds a freshly built declaration into an existing one. The address is
// the seam: Terraform owns type, attributes, the group a node sits in and a
// schedule's traffic; people own ids, assumptions, load and other traffic,
// notes, positions and edges. Nodes that left Terraform are kept and
// marked stale, never deleted.
func Merge(existing, fresh model.Spec) (model.Spec, []string) {
	m := newMerger(existing)
	m.mergeHeader(fresh)
	m.groupID = mergeGroups(existing.Groups, fresh.Groups, &m.out)
	for _, f := range fresh.Nodes {
		m.mergeNode(f)
	}
	m.markStale(existing.Nodes)
	m.mergeEdges(existing.Edges, fresh.Edges)
	m.out.Groups = usedGroups(m.out.Groups, m.out.Nodes)
	return m.out, m.warnings
}

// merger carries the state of one Merge: the declaration being built and
// the bookkeeping that ties fresh nodes to the ones people already have.
type merger struct {
	out      model.Spec
	warnings []string
	byAddr   map[string]int    // address -> index of the existing node
	used     map[string]bool   // node ids taken in out
	seen     map[int]bool      // existing nodes Terraform still declares
	rename   map[string]string // fresh id -> final id
	groupID  map[string]string // fresh group id -> final group id
}

func newMerger(existing model.Spec) *merger {
	m := &merger{
		out:    existing,
		byAddr: map[string]int{},
		used:   map[string]bool{},
		seen:   map[int]bool{},
		rename: map[string]string{},
	}
	for i, n := range existing.Nodes {
		if n.Address != "" {
			m.byAddr[n.Address] = i
		}
		m.used[n.ID] = true
	}
	m.out.Nodes = append([]model.Node(nil), existing.Nodes...)
	return m
}

// mergeHeader fills the region and name people left empty; a region that
// disagrees stays as people wrote it.
func (m *merger) mergeHeader(fresh model.Spec) {
	if m.out.Region == "" {
		m.out.Region = fresh.Region
	} else if fresh.Region != "" && fresh.Region != m.out.Region {
		m.warn("Terraform region %s differs from the declaration's %s; kept %s", fresh.Region, m.out.Region, m.out.Region)
	}
	if m.out.Name == "" {
		m.out.Name = fresh.Name
	}
}

// mergeNode places one fresh node: a generated entry reuses its id, a known
// address updates the node people have, and anything else is added under a
// free id.
func (m *merger) mergeNode(f model.Node) {
	if f.Address == "" {
		m.addGenerated(f)
		return
	}
	if i, ok := m.byAddr[f.Address]; ok {
		m.seen[i] = true
		n := &m.out.Nodes[i]
		updateNode(n, f, m.groupID[f.Group])
		m.rename[f.ID] = n.ID
		return
	}
	m.addNew(f)
}

// addGenerated keeps a generated entry under its own id, reusing a node that
// already has it.
func (m *merger) addGenerated(f model.Node) {
	m.rename[f.ID] = f.ID
	if !m.used[f.ID] {
		m.out.Nodes = append(m.out.Nodes, f)
		m.used[f.ID] = true
	}
}

// addNew adds a node Terraform declares for the first time, numbering its id
// when people already use it.
func (m *merger) addNew(f model.Node) {
	orig, id := f.ID, f.ID
	for i := 2; m.used[id]; i++ {
		id = fmt.Sprintf("%s-%d", orig, i)
	}
	m.used[id] = true
	f.ID = id
	f.Group = m.groupID[f.Group]
	m.rename[orig] = id
	m.out.Nodes = append(m.out.Nodes, f)
}

// updateNode takes what Terraform owns from f into n and fills only the
// assumptions, load and traffic people left out.
func updateNode(n *model.Node, f model.Node, group string) {
	n.Type, n.Attributes, n.Stale, n.Group = f.Type, f.Attributes, false, group
	// A count Terraform knows is Terraform's; one it cannot know before
	// apply keeps the count people wrote.
	if f.Instances != model.UnknownInstances || n.Instances <= 0 {
		n.Instances = f.Instances
	}
	for k, v := range f.Assumptions {
		if _, has := n.Assumptions[k]; has {
			continue
		}
		if n.Assumptions == nil {
			n.Assumptions = map[string]any{}
		}
		n.Assumptions[k] = v
	}
	if n.Load == nil {
		n.Load = f.Load
	}
	// A schedule from Terraform follows Terraform, unless people gave
	// the node a load or traffic of another shape.
	if f.Traffic != nil && n.Load == nil && (n.Traffic == nil || n.Traffic.Schedule != "") {
		n.Traffic = f.Traffic
	}
}

// markStale flags the addressed nodes Terraform no longer declares.
func (m *merger) markStale(existing []model.Node) {
	for i, n := range existing {
		if n.Address != "" && !m.seen[i] {
			m.out.Nodes[i].Stale = true
			m.warn("%s (%s) is no longer in Terraform; kept and marked stale", n.ID, n.Address)
		}
	}
}

// mergeEdges keeps people's edges and adds the fresh ones, renamed to the
// final ids, that do not repeat a pair already drawn.
func (m *merger) mergeEdges(existing, fresh []model.Edge) {
	pairs := map[[2]string]bool{}
	for _, e := range existing {
		pairs[[2]string{e.From, e.To}] = true
	}
	m.out.Edges = append([]model.Edge(nil), existing...)
	for _, e := range fresh {
		from, to := m.rename[e.From], m.rename[e.To]
		if from == "" || to == "" || pairs[[2]string{from, to}] {
			continue
		}
		pairs[[2]string{from, to}] = true
		e.From, e.To = from, to
		m.out.Edges = append(m.out.Edges, e)
	}
}

func (m *merger) warn(format string, args ...any) {
	m.warnings = append(m.warnings, fmt.Sprintf(format, args...))
}

// mergeGroups adds the fresh groups to out and maps each fresh id to its id
// there: the same kind and label is the same group, and a different group
// with a taken id gets a new one.
func mergeGroups(existing, fresh []model.Group, out *model.Spec) map[string]string {
	ids := map[string]string{"": ""}
	byID := map[string]model.Group{}
	for _, g := range existing {
		byID[g.ID] = g
	}
	out.Groups = append([]model.Group(nil), existing...)
	for _, f := range fresh {
		orig := f.ID
		id := sameGroup(out.Groups, f)
		if id == "" {
			id = f.ID
			for i := 2; byID[id].ID != ""; i++ {
				id = fmt.Sprintf("%s-%d", f.ID, i)
			}
			f.ID = id
			byID[id] = f
			out.Groups = append(out.Groups, f)
		}
		ids[orig] = id
	}
	return ids
}

// sameGroup returns the id of the group with f's kind and label, or "".
func sameGroup(groups []model.Group, f model.Group) string {
	for _, g := range groups {
		if g.Kind == f.Kind && g.Label == f.Label {
			return g.ID
		}
	}
	return ""
}

// usedGroups keeps the groups some node sits in; a boundary with nothing in
// it draws nothing.
func usedGroups(groups []model.Group, nodes []model.Node) []model.Group {
	in := map[string]bool{}
	for _, n := range nodes {
		in[n.Group] = true
	}
	var out []model.Group
	for _, g := range groups {
		if in[g.ID] {
			out = append(out, g)
		}
	}
	return out
}
