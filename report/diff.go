package report

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/O6lvl4/archgopher/engine"
	"github.com/O6lvl4/archgopher/meter"
)

// Marker opens every diff comment, so a bot can find and update its own.
const Marker = "<!-- archgopher diff -->"

// Tight is the headroom under which a node is called out: less than a fifth
// of its capacity left at peak.
const Tight = 0.2

// Diff is what changed between two readings of one architecture.
type Diff struct {
	Name      string       `json:"name"`
	BeforeUSD float64      `json:"beforeUsd"`
	AfterUSD  float64      `json:"afterUsd"`
	Nodes     []NodeChange `json:"nodes"`
	Paths     []PathChange `json:"paths"`
	// Alerts are changes a reviewer should not miss: a node over or near
	// its capacity, a node that no longer reads, prices that became unknown.
	Alerts []string `json:"alerts"`
}

// DeltaUSD is the monthly change.
func (d Diff) DeltaUSD() float64 { return d.AfterUSD - d.BeforeUSD }

// Empty reports a diff with nothing to show.
func (d Diff) Empty() bool { return len(d.Nodes) == 0 && len(d.Paths) == 0 && len(d.Alerts) == 0 }

// NodeChange is one node that was added, removed or read differently.
type NodeChange struct {
	ID string `json:"id"`
	// Change is "added", "removed" or "changed".
	Change         string       `json:"change"`
	Label          string       `json:"label"`
	BeforeUSD      float64      `json:"beforeUsd"`
	AfterUSD       float64      `json:"afterUsd"`
	BeforeHeadroom *float64     `json:"beforeHeadroom,omitempty"`
	AfterHeadroom  *float64     `json:"afterHeadroom,omitempty"`
	Lines          []LineChange `json:"lines,omitempty"`
}

// LineChange is one cost line of a node, before and after; nil is absent.
type LineChange struct {
	Name   string   `json:"name"`
	Unit   string   `json:"unit"`
	Before *float64 `json:"beforeUsd"`
	After  *float64 `json:"afterUsd"`
}

// PathChange is one path whose latency or availability moved, or that
// appeared or disappeared.
type PathChange struct {
	Path   string             `json:"path"`
	Change string             `json:"change"`
	Before *engine.PathResult `json:"before,omitempty"`
	After  *engine.PathResult `json:"after,omitempty"`
}

// Compare reads what changed from before to after. Nodes and paths are
// matched by ID, which Terraform merges keep stable.
func Compare(before, after engine.Result) Diff {
	d := Diff{Name: after.Name, BeforeUSD: before.MonthlyUSD, AfterUSD: after.MonthlyUSD}
	b, a := byID(before), byID(after)
	for _, id := range union(keys(b), keys(a)) {
		p := pair{id: id}
		p.before, p.inBefore = b[id]
		p.after, p.inAfter = a[id]
		if c := p.change(); c.Change != "changed" || len(c.Lines) > 0 || moved(c.BeforeHeadroom, c.AfterHeadroom, 0.001) {
			d.Nodes = append(d.Nodes, c)
		}
		d.Alerts = append(d.Alerts, p.alerts()...)
	}
	sort.SliceStable(d.Nodes, func(i, j int) bool {
		return math.Abs(d.Nodes[i].AfterUSD-d.Nodes[i].BeforeUSD) > math.Abs(d.Nodes[j].AfterUSD-d.Nodes[j].BeforeUSD)
	})
	d.Paths = pathChanges(before.Paths, after.Paths)
	if after.UnpricedCosts > before.UnpricedCosts {
		d.Alerts = append(d.Alerts, fmt.Sprintf("%d more cost lines have unknown prices", after.UnpricedCosts-before.UnpricedCosts))
	}
	return d
}

// pair is one node's readings before and after, with whether it was there.
type pair struct {
	id                string
	before, after     engine.NodeResult
	inBefore, inAfter bool
}

func (p pair) change() NodeChange {
	c := NodeChange{ID: p.id}
	nb, na := p.before, p.after
	switch {
	case !p.inBefore:
		c.Change, c.Label, c.AfterUSD, c.AfterHeadroom = "added", label(na), na.MonthlyUSD, na.MinHeadroom()
	case !p.inAfter:
		c.Change, c.Label, c.BeforeUSD, c.BeforeHeadroom = "removed", label(nb), nb.MonthlyUSD, nb.MinHeadroom()
	default:
		c.Change, c.Label = "changed", label(na)
		c.BeforeUSD, c.AfterUSD = nb.MonthlyUSD, na.MonthlyUSD
		c.BeforeHeadroom, c.AfterHeadroom = nb.MinHeadroom(), na.MinHeadroom()
	}
	c.Lines = lineChanges(nb.Costs, na.Costs)
	return c
}

// alerts call out a node that stopped reading, or that crossed into tight
// headroom or over its capacity.
func (p pair) alerts() []string {
	if !p.inAfter {
		return nil
	}
	var out []string
	if p.after.Error != "" && (!p.inBefore || p.before.Error == "") {
		out = append(out, fmt.Sprintf("`%s` no longer reads: %s", p.id, p.after.Error))
	}
	var hb *float64
	if p.inBefore {
		hb = p.before.MinHeadroom()
	}
	if a, ok := headroomAlert(p.id, hb, p.after.MinHeadroom()); ok {
		out = append(out, a)
	}
	return out
}

// headroomAlert is set when headroom h falls below zero or below Tight and
// the headroom before, hb, was not already there.
func headroomAlert(id string, hb, h *float64) (string, bool) {
	switch {
	case h == nil:
		return "", false
	case *h < 0 && (hb == nil || *hb >= 0):
		return fmt.Sprintf("`%s` is over capacity at peak (headroom %s)", id, pct(h)), true
	case *h >= 0 && *h < Tight && (hb == nil || *hb >= Tight):
		return fmt.Sprintf("`%s` has less than %s headroom at peak (%s)", id, pct(ptr(Tight)), pct(h)), true
	}
	return "", false
}

func byID(r engine.Result) map[string]engine.NodeResult {
	out := map[string]engine.NodeResult{}
	for _, n := range members(r) {
		out[n.ID] = n
	}
	return out
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func union(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, k := range append(a, b...) {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

type lineKey struct{ name, unit string }

// lineChanges pairs cost lines by name and unit and keeps the ones that
// moved by a cent or more, or whose price became known or unknown.
func lineChanges(before, after []meter.Cost) []LineChange {
	var order []lineKey
	seen := map[lineKey]bool{}
	sum := func(lines []meter.Cost) map[lineKey]*float64 {
		m := map[lineKey]*float64{}
		for _, c := range lines {
			k := lineKey{c.Name, c.Unit}
			if !seen[k] {
				seen[k] = true
				order = append(order, k)
			}
			m[k] = plus(m[k], c.MonthlyUSD)
		}
		return m
	}
	b, a := sum(before), sum(after)
	var out []LineChange
	for _, k := range order {
		vb, inB := b[k]
		va, inA := a[k]
		if inB && inA && !moved(vb, va, 0.005) {
			continue
		}
		out = append(out, LineChange{Name: k.name, Unit: k.unit, Before: vb, After: va})
	}
	return out
}

// plus adds a line's amount to the sum so far; a line of unknown price makes
// the sum unknown until a later known line starts it again.
func plus(sum, v *float64) *float64 {
	if v == nil || sum == nil {
		return v
	}
	total := *sum + *v
	return &total
}

// moved reports two optional numbers that differ by at least eps, or of which
// only one is known.
func moved(b, a *float64, eps float64) bool {
	if (b == nil) != (a == nil) {
		return true
	}
	return b != nil && math.Abs(*a-*b) >= eps
}

func ptr(v float64) *float64 { return &v }

func pathChanges(before, after []engine.PathResult) []PathChange {
	b, a := pathsByKey(before), pathsByKey(after)
	var out []PathChange
	for _, k := range union(keys(b), keys(a)) {
		pb, inB := b[k]
		pa, inA := a[k]
		switch {
		case !inB:
			out = append(out, PathChange{Path: k, Change: "added", After: &pa})
		case !inA:
			out = append(out, PathChange{Path: k, Change: "removed", Before: &pb})
		case math.Abs(pa.P99Ms-pb.P99Ms) >= 1 || math.Abs(pa.Availability-pb.Availability) >= 1e-6:
			out = append(out, PathChange{Path: k, Change: "changed", Before: &pb, After: &pa})
		}
	}
	return out
}

func pathsByKey(paths []engine.PathResult) map[string]engine.PathResult {
	out := map[string]engine.PathResult{}
	for _, p := range paths {
		out[strings.Join(p.Nodes, " → ")] = p
	}
	return out
}
