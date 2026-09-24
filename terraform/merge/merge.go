// Package merge folds a declaration built from Terraform into one people
// have edited. The Terraform address is the seam.
package merge

import (
	"fmt"

	"github.com/O6lvl4/archgopher/model"
)

// Merge folds a freshly built declaration into an existing one. The address is
// the seam: Terraform owns type, attributes and the group a node sits in;
// people own ids, assumptions, load, notes, positions and edges. Nodes that left Terraform are kept and
// marked stale, never deleted.
func Merge(existing, fresh model.Spec) (model.Spec, []string) {
	var warnings []string
	out := existing
	if out.Region == "" {
		out.Region = fresh.Region
	} else if fresh.Region != "" && fresh.Region != out.Region {
		warnings = append(warnings, fmt.Sprintf("Terraform region %s differs from the declaration's %s; kept %s", fresh.Region, out.Region, out.Region))
	}
	if out.Name == "" {
		out.Name = fresh.Name
	}
	byAddr := map[string]int{}
	used := map[string]bool{}
	for i, n := range existing.Nodes {
		if n.Address != "" {
			byAddr[n.Address] = i
		}
		used[n.ID] = true
	}
	out.Nodes = append([]model.Node(nil), existing.Nodes...)
	groupID := mergeGroups(existing.Groups, fresh.Groups, &out)
	rename := map[string]string{} // fresh id -> final id
	seen := map[int]bool{}
	for _, f := range fresh.Nodes {
		if f.Address == "" {
			// A generated entry: reuse a node with the same id.
			rename[f.ID] = f.ID
			if !used[f.ID] {
				out.Nodes = append(out.Nodes, f)
				used[f.ID] = true
			}
			continue
		}
		if i, ok := byAddr[f.Address]; ok {
			seen[i] = true
			n := &out.Nodes[i]
			n.Type, n.Attributes, n.Stale, n.Group = f.Type, f.Attributes, false, groupID[f.Group]
			for k, v := range f.Assumptions {
				if _, has := n.Assumptions[k]; !has {
					if n.Assumptions == nil {
						n.Assumptions = map[string]any{}
					}
					n.Assumptions[k] = v
				}
			}
			if n.Load == nil {
				n.Load = f.Load
			}
			rename[f.ID] = n.ID
			continue
		}
		orig, id := f.ID, f.ID
		for i := 2; used[id]; i++ {
			id = fmt.Sprintf("%s-%d", orig, i)
		}
		used[id] = true
		f.ID = id
		f.Group = groupID[f.Group]
		rename[orig] = id
		out.Nodes = append(out.Nodes, f)
	}
	for i, n := range existing.Nodes {
		if n.Address != "" && !seen[i] {
			out.Nodes[i].Stale = true
			warnings = append(warnings, fmt.Sprintf("%s (%s) is no longer in Terraform; kept and marked stale", n.ID, n.Address))
		}
	}
	pairs := map[[2]string]bool{}
	for _, e := range existing.Edges {
		pairs[[2]string{e.From, e.To}] = true
	}
	out.Edges = append([]model.Edge(nil), existing.Edges...)
	for _, e := range fresh.Edges {
		from, to := rename[e.From], rename[e.To]
		if from == "" || to == "" || pairs[[2]string{from, to}] {
			continue
		}
		pairs[[2]string{from, to}] = true
		e.From, e.To = from, to
		out.Edges = append(out.Edges, e)
	}
	out.Groups = usedGroups(out.Groups, out.Nodes)
	return out, warnings
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
		orig, id := f.ID, ""
		for _, g := range out.Groups {
			if g.Kind == f.Kind && g.Label == f.Label {
				id = g.ID
				break
			}
		}
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
