// Package review reads the shape of a declaration against practices a
// provider knows: where the graph departs from the usual way of building it.
// A finding is not an error. A design may depart on purpose; the finding asks
// for the reason to be written down. Providers supply the rules; this package
// knows no provider.
package review

import (
	"fmt"
	"sort"
	"strings"

	"github.com/O6lvl4/archgopher/model"
)

// Rule is one practice.
type Rule struct {
	ID string `json:"id"`
	// Pillar is the Well-Architected pillar, or the lens, the practice comes from.
	Pillar string `json:"pillar"`
	Title  string `json:"title"`
	// Hint says when departing is fine and what to do otherwise.
	Hint string `json:"hint"`
	// Source is where the practice is written.
	Source string `json:"source"`
	// FanIn is the shape the rule looks for.
	FanIn *FanIn `json:"fanIn,omitempty"`
}

// FanIn finds a node of type Node fed by more than Max resources of type
// From, counting a node's instances: one function consuming two queues.
type FanIn struct {
	Node string `json:"node"`
	From string `json:"from"`
	Max  int    `json:"max"`
}

// Finding is one place the declaration departs from a rule.
type Finding struct {
	Rule    string `json:"rule"`
	Pillar  string `json:"pillar"`
	Node    string `json:"node"`
	Message string `json:"message"`
	// From lists the nodes that make up the shape.
	From   []string `json:"from,omitempty"`
	Hint   string   `json:"hint"`
	Source string   `json:"source"`
}

// Find applies every rule to a declaration with its patterns expanded.
func Find(spec model.Spec, rules []Rule) []Finding {
	byID := map[string]model.Node{}
	for _, n := range spec.Nodes {
		byID[n.ID] = n
	}
	var out []Finding
	for _, r := range rules {
		if r.FanIn != nil {
			out = append(out, fanIn(spec, byID, r)...)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Node != out[j].Node {
			return out[i].Node < out[j].Node
		}
		return out[i].Rule < out[j].Rule
	})
	return out
}

func fanIn(spec model.Spec, byID map[string]model.Node, r Rule) []Finding {
	f := r.FanIn
	feeds := map[string][]string{}
	for _, e := range spec.Edges {
		to, from := byID[e.To], byID[e.From]
		if to.Type == f.Node && from.Type == f.From && !contains(feeds[e.To], e.From) {
			feeds[e.To] = append(feeds[e.To], e.From)
		}
	}
	var out []Finding
	for _, id := range sortedKeys(feeds) {
		count := 0
		for _, from := range feeds[id] {
			count += byID[from].Count()
		}
		if count <= f.Max {
			continue
		}
		sort.Strings(feeds[id])
		out = append(out, Finding{
			Rule:    r.ID,
			Pillar:  r.Pillar,
			Node:    id,
			Message: fmt.Sprintf("%s: fed by %d %s (%s)", r.Title, count, f.From, describe(feeds[id], byID)),
			From:    feeds[id],
			Hint:    r.Hint,
			Source:  r.Source,
		})
	}
	return out
}

// describe names the feeding nodes, with their instances when more than one.
func describe(ids []string, byID map[string]model.Node) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = id
		if c := byID[id].Count(); c > 1 {
			parts[i] = fmt.Sprintf("%s ×%d", id, c)
		}
	}
	return strings.Join(parts, ", ")
}

// Text renders findings for a terminal.
func Text(fs []Finding) string {
	if len(fs) == 0 {
		return "No departures from the known practices.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d departure(s) from the known practices. Fix them, or write down why the design departs.\n", len(fs))
	for _, f := range fs {
		fmt.Fprintf(&b, "\n- [%s] %s\n  %s\n  hint: %s\n  source: %s\n", f.Pillar, f.Node, f.Message, f.Hint, f.Source)
	}
	return b.String()
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
