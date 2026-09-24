// Package merge folds a declaration built from Terraform into one people
// have edited. The Terraform address is the seam.
package merge

import (
	"fmt"

	"github.com/O6lvl4/archgopher/model"
)

// Merge folds a freshly built declaration into an existing one. The address is
// the seam: Terraform owns type and attributes; people own ids, assumptions,
// load, notes, positions and edges. Nodes that left Terraform are kept and
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
			n.Type, n.Attributes, n.Stale = f.Type, f.Attributes, false
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
	return out, warnings
}
