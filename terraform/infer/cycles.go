package infer

import (
	"fmt"

	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/scouter"
)

// frontDoors adds the users entry and its edges to every front door that
// nothing inside the graph calls, and to what front door aliases expose.
func (b *builder) frontDoors(nodes []model.Node, edges []edgeKey) ([]model.Node, []edgeKey) {
	incoming := map[string]bool{}
	for _, e := range edges {
		incoming[e.to] = true
	}
	doors := map[string]bool{}
	for _, n := range nodes {
		if b.rules.FrontDoors[n.Type] && !incoming[n.Address] {
			doors[n.Address] = true
		}
	}
	for _, r := range b.ev.Resources {
		if path, ok := b.rules.FrontDoorAliases[r.Type]; ok {
			for _, t := range b.targetsAt(r, path) {
				doors[t] = true
			}
		}
	}
	if len(doors) == 0 {
		return nodes, edges
	}
	entry := model.Node{ID: UsersID, Type: scouter.EntryType, Note: "Users reaching the front doors. Set load: monthly volume and peak per second."}
	nodes = append([]model.Node{entry}, nodes...)
	for _, addr := range sortedKeys(doors) {
		edges = append(edges, edgeKey{from: UsersID, to: addr})
	}
	return nodes, edges
}

// breakCycles drops edges that close a cycle (a callback URL, mutual references)
// and reports them; the engine refuses cyclic graphs.
func (b *builder) breakCycles(nodes []model.Node, edges []edgeKey) []model.Edge {
	adj := map[string][]string{}
	var out []model.Edge
	for _, e := range edges {
		from, to := b.nodeID(e.from), b.nodeID(e.to)
		if reaches(adj, to, from) {
			b.warnings = append(b.warnings, fmt.Sprintf("dropped edge %s -> %s: it closes a cycle", from, to))
			continue
		}
		adj[from] = append(adj[from], to)
		out = append(out, model.Edge{From: from, To: to, Kind: e.kind})
	}
	return oneEdgePerPair(out)
}

// nodeID is the id of the node at addr; the users entry is its own address.
func (b *builder) nodeID(addr string) string {
	if addr == UsersID {
		return UsersID
	}
	return b.ids[addr]
}

// reaches reports whether to can be reached from from along adj.
func reaches(adj map[string][]string, from, to string) bool {
	seen := map[string]bool{}
	stack := []string{from}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n == to {
			return true
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		stack = append(stack, adj[n]...)
	}
	return false
}

// oneEdgePerPair folds edges between the same two nodes into one whose
// operations are their kinds: a role that reads and writes a table is one
// edge that does both.
func oneEdgePerPair(edges []model.Edge) []model.Edge {
	var out []model.Edge
	at := map[[2]string]int{}
	for _, e := range edges {
		k := [2]string{e.From, e.To}
		i, seen := at[k]
		if !seen {
			at[k] = len(out)
			out = append(out, e)
			continue
		}
		first := &out[i]
		if len(first.Ops) == 0 {
			first.Ops = []model.Op{{Kind: first.Kind}}
			first.Kind = ""
		}
		first.Ops = append(first.Ops, model.Op{Kind: e.Kind})
	}
	return out
}
