package engine

import (
	"fmt"

	"github.com/O6lvl4/archgopher/scouter"
)

// paths lists every path from a node that brings in load to a sink, up to MaxPaths.
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
