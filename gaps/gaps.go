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
	// Instances: a count or for_each is not known before apply, so one
	// resource is read.
	Instances Kind = "instances"
	// Failed: the node could not be read for another reason.
	Failed Kind = "failed"
)

var order = map[Kind]int{Load: 0, Caller: 1, Assumption: 2, Instances: 3, Ratio: 4, Failed: 5}

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
	j := judge{books: books, region: expanded.Region, fed: map[string]bool{}, failed: map[string]string{}}
	for _, e := range expanded.Edges {
		j.fed[e.To] = true
	}
	for _, n := range res.Nodes {
		if n.Error != "" {
			j.failed[n.ID] = n.Error
		}
	}
	var out []Gap
	for _, n := range expanded.Nodes {
		if s, ok := reg[n.Type]; ok {
			out = append(out, j.nodeGaps(n, s)...)
		}
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

// judge decides the gaps of the nodes of one expanded declaration.
type judge struct {
	books  book.Books
	region string
	fed    map[string]bool   // nodes some edge sends work to
	failed map[string]string // the engine's error per node
}

func (j judge) nodeGaps(n model.Node, s scouter.Scouter) []Gap {
	var out []Gap
	unfedEntry := s.Meta().Type == scouter.EntryType && n.Load == nil && n.Traffic == nil
	if g, ok := j.sourceGap(n, s); ok {
		out = append(out, g)
	}
	missing := assumptionGaps(n, s)
	out = append(out, missing...)
	if n.Instances == model.UnknownInstances {
		out = append(out, Gap{Kind: Instances, Node: n.ID, Message: "count or for_each is not known before apply; one is read",
			Hint: "set instances to how many there will be"})
	}
	if failure := j.failed[n.ID]; failure != "" && len(missing) == 0 && !unfedEntry {
		out = append(out, Gap{Kind: Failed, Node: n.ID, Message: failure})
	}
	return out
}

// sourceGap is an entry without load, or a node that takes work nobody sends.
func (j judge) sourceGap(n model.Node, s scouter.Scouter) (Gap, bool) {
	if n.Load != nil || n.Traffic != nil {
		return Gap{}, false
	}
	if s.Meta().Type == scouter.EntryType {
		return Gap{Kind: Load, Node: n.ID, Message: "no load",
			Hint: "set load or traffic from access logs, analytics or the plan"}, true
	}
	if !j.fed[n.ID] && j.takesWork(n, s) {
		return Gap{Kind: Caller, Node: n.ID, Message: "nothing sends it work",
			Hint: "add edges for calls the infrastructure code does not show (application code, roles made elsewhere, other accounts), or give it a load"}, true
	}
	return Gap{}, false
}

// assumptionGaps are the required assumptions with no default that the node leaves unset.
func assumptionGaps(n model.Node, s scouter.Scouter) []Gap {
	var out []Gap
	for _, f := range s.Assumptions() {
		if !f.Required || f.Default != nil {
			continue
		}
		if v, ok := n.Assumptions[f.Key]; ok && v != nil {
			continue
		}
		label := f.Label
		if f.Unit != "" {
			label += " (" + f.Unit + ")"
		}
		out = append(out, Gap{Kind: Assumption, Node: n.ID, Key: f.Key, Message: label + " is unknown", Hint: f.Hint})
	}
	return out
}

// takesWork says whether the node's readings change with the work it
// receives: it is read once idle and once busy. Alarms, parameters and other
// fixed-price nodes accept edges but read the same either way.
func (j judge) takesWork(n model.Node, s scouter.Scouter) bool {
	kinds := s.Meta().Kinds
	if len(kinds) == 0 {
		return false
	}
	trial := withTrialNumbers(n, s)
	busy := model.Demand{}
	for _, k := range kinds {
		busy[k] = model.Load{Monthly: 1e6, PeakPerSecond: 10}
	}
	idle, errIdle := j.readings(trial, s, model.Demand{})
	working, errBusy := j.readings(trial, s, busy)
	if errIdle != nil || errBusy != nil {
		return true
	}
	return idle != working
}

// withTrialNumbers is a copy of n whose unknown numbers are set to 1, so a
// node that is missing them still answers a trial reading.
func withTrialNumbers(n model.Node, s scouter.Scouter) model.Node {
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
	return trial
}

// readings is a node's cost quantities and limit demands, as one comparable string.
func (j judge) readings(n model.Node, s scouter.Scouter, d model.Demand) (string, error) {
	r := meter.NewRecorder(j.region, j.books)
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
