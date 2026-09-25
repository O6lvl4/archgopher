// Package gaps lists what a declaration does not know yet: the numbers and
// calls that infrastructure code cannot show and someone (a person or an
// agent reading the application and its metrics) has to fill in.
package gaps

import (
	"fmt"
	"sort"
	"strings"

	"github.com/O6lvl4/archgopher/book"
	"github.com/O6lvl4/archgopher/engine"
	"github.com/O6lvl4/archgopher/field"
	"github.com/O6lvl4/archgopher/meter"
	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/scouter"
)

// Kind is what is missing.
type Kind string

const (
	// Load: an entry with no volume, so nothing downstream is read.
	Load Kind = "load"
	// Caller: a node that takes work but nothing sends it any. The calls may
	// come from outside the infrastructure code: application code, roles
	// made elsewhere, another account or project.
	Caller Kind = "caller"
	// Assumption: a required number is unknown.
	Assumption Kind = "assumption"
	// Ratio: an edge says neither how many calls one upstream unit makes nor
	// where its one-per-unit came from.
	Ratio Kind = "ratio"
	// Failed: the node could not be read for another reason.
	Failed Kind = "failed"
)

var order = map[Kind]int{Load: 0, Caller: 1, Assumption: 2, Ratio: 3, Failed: 4}

// Gap is one unknown.
type Gap struct {
	Kind Kind `json:"kind"`
	// Node is set for node gaps; From and To for edge gaps.
	Node string `json:"node,omitempty"`
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	// Key is the assumption key of an assumption gap.
	Key     string `json:"key,omitempty"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// Find lists the gaps of a spec as written, given the spec its patterns
// expand to and the engine's reading of that. Nodes are judged expanded, so a
// pattern's members count; ratios only on the edges someone can edit.
func Find(spec, expanded model.Spec, reg scouter.Registry, books book.Books, res engine.Result) []Gap {
	fed := map[string]bool{}
	for _, e := range expanded.Edges {
		fed[e.To] = true
	}
	failed := map[string]string{}
	for _, n := range res.Nodes {
		if n.Error != "" {
			failed[n.ID] = n.Error
		}
	}
	var out []Gap
	for _, n := range expanded.Nodes {
		s, ok := reg[n.Type]
		if !ok {
			continue
		}
		out = append(out, nodeGaps(n, s, books, expanded.Region, fed[n.ID], failed[n.ID])...)
	}
	for _, e := range spec.Edges {
		if len(e.Ops) == 0 && e.PerUnit == nil && e.Note == "" {
			out = append(out, Gap{
				Kind: Ratio, From: e.From, To: e.To,
				Message: "one call per upstream unit is assumed",
				Hint:    "set perUnit (or ops) from the code and the measured counts, or a note saying why one per unit holds",
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return order[out[i].Kind] < order[out[j].Kind] })
	return out
}

func nodeGaps(n model.Node, s scouter.Scouter, books book.Books, region string, fed bool, failure string) []Gap {
	meta := s.Meta()
	var out []Gap
	sourced := n.Load != nil || n.Traffic != nil
	switch {
	case meta.Type == scouter.EntryType && !sourced:
		out = append(out, Gap{Kind: Load, Node: n.ID, Message: "no load",
			Hint: "set load or traffic from access logs, analytics or the plan"})
	case meta.Type != scouter.EntryType && !fed && !sourced && takesWork(n, s, books, region):
		out = append(out, Gap{Kind: Caller, Node: n.ID, Message: "nothing sends it work",
			Hint: "add edges for calls the infrastructure code does not show (application code, roles made elsewhere, other accounts), or give it a load"})
	}
	missing := 0
	for _, f := range s.Assumptions() {
		if !f.Required || f.Default != nil {
			continue
		}
		if v, ok := n.Assumptions[f.Key]; ok && v != nil {
			continue
		}
		missing++
		label := f.Label
		if f.Unit != "" {
			label += " (" + f.Unit + ")"
		}
		out = append(out, Gap{Kind: Assumption, Node: n.ID, Key: f.Key, Message: label + " is unknown", Hint: f.Hint})
	}
	if failure != "" && missing == 0 && !(meta.Type == scouter.EntryType && !sourced) {
		out = append(out, Gap{Kind: Failed, Node: n.ID, Message: failure})
	}
	return out
}

// takesWork says whether the node's readings change with the work it
// receives: it is read once idle and once busy. Alarms, parameters and other
// fixed-price nodes accept edges but read the same either way. Unknown numbers
// are set to 1 for the trial so a node that is missing them still answers.
func takesWork(n model.Node, s scouter.Scouter, books book.Books, region string) bool {
	kinds := s.Meta().Kinds
	if len(kinds) == 0 {
		return false
	}
	trial := n
	trial.Assumptions = map[string]any{}
	for k, v := range n.Assumptions {
		trial.Assumptions[k] = v
	}
	for _, f := range s.Assumptions() {
		if v, ok := trial.Assumptions[f.Key]; f.Type == field.Number && f.Default == nil && (!ok || v == nil) {
			trial.Assumptions[f.Key] = 1.0
		}
	}
	busy := model.Demand{}
	for _, k := range kinds {
		busy[k] = model.Load{Monthly: 1e6, PeakPerSecond: 10}
	}
	idle, errIdle := readings(trial, s, books, region, model.Demand{})
	working, errBusy := readings(trial, s, books, region, busy)
	if errIdle != nil || errBusy != nil {
		return true
	}
	return idle != working
}

// readings is a node's cost quantities and limit demands, as one comparable string.
func readings(n model.Node, s scouter.Scouter, books book.Books, region string, d model.Demand) (string, error) {
	r := meter.NewRecorder(region, books)
	if err := s.Scout(n, d, r); err != nil {
		return "", err
	}
	var b strings.Builder
	for _, c := range r.Costs() {
		fmt.Fprintf(&b, "%s=%g;", c.Name, c.Quantity)
	}
	for _, l := range r.Limits() {
		fmt.Fprintf(&b, "%s=%g;", l.Name, l.Demand)
	}
	return b.String(), nil
}

// Text writes gaps one per line, grouped by kind.
func Text(gs []Gap) string {
	if len(gs) == 0 {
		return "No gaps.\n"
	}
	var b strings.Builder
	var last Kind
	for _, g := range gs {
		if g.Kind != last {
			if last != "" {
				b.WriteString("\n")
			}
			fmt.Fprintf(&b, "%s\n", g.Kind)
			last = g.Kind
		}
		where := g.Node
		if g.From != "" {
			where = g.From + " -> " + g.To
		}
		fmt.Fprintf(&b, "  %s: %s\n", where, g.Message)
	}
	return b.String()
}
