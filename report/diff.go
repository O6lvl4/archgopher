package report

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
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
		nb, inB := b[id]
		na, inA := a[id]
		c := NodeChange{ID: id}
		switch {
		case !inB:
			c.Change, c.Label, c.AfterUSD, c.AfterHeadroom = "added", label(na), na.MonthlyUSD, na.MinHeadroom()
		case !inA:
			c.Change, c.Label, c.BeforeUSD, c.BeforeHeadroom = "removed", label(nb), nb.MonthlyUSD, nb.MinHeadroom()
		default:
			c.Change, c.Label = "changed", label(na)
			c.BeforeUSD, c.AfterUSD = nb.MonthlyUSD, na.MonthlyUSD
			c.BeforeHeadroom, c.AfterHeadroom = nb.MinHeadroom(), na.MinHeadroom()
		}
		c.Lines = lineChanges(nb.Costs, na.Costs)
		if c.Change != "changed" || len(c.Lines) > 0 || moved(c.BeforeHeadroom, c.AfterHeadroom, 0.001) {
			d.Nodes = append(d.Nodes, c)
		}
		d.Alerts = append(d.Alerts, alerts(id, nb, inB, na, inA)...)
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

// lineChanges pairs cost lines by name and unit and keeps the ones that
// moved by a cent or more, or whose price became known or unknown.
func lineChanges(before, after []meter.Cost) []LineChange {
	type key struct{ name, unit string }
	b, a := map[key]*float64{}, map[key]*float64{}
	var order []key
	add := func(m map[key]*float64, lines []meter.Cost) {
		for _, c := range lines {
			k := key{c.Name, c.Unit}
			if _, seen := b[k]; !seen {
				if _, seen := a[k]; !seen {
					order = append(order, k)
				}
			}
			v := c.MonthlyUSD
			if v != nil {
				if old, ok := m[k]; ok && old != nil {
					sum := *old + *v
					v = &sum
				}
			}
			m[k] = v
		}
	}
	add(b, before)
	add(a, after)
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

// moved reports two optional numbers that differ by at least eps, or of which
// only one is known.
func moved(b, a *float64, eps float64) bool {
	if (b == nil) != (a == nil) {
		return true
	}
	return b != nil && math.Abs(*a-*b) >= eps
}

func alerts(id string, nb engine.NodeResult, inB bool, na engine.NodeResult, inA bool) []string {
	if !inA {
		return nil
	}
	var out []string
	if na.Error != "" && (!inB || nb.Error == "") {
		out = append(out, fmt.Sprintf("`%s` no longer reads: %s", id, na.Error))
	}
	h := na.MinHeadroom()
	var hb *float64
	if inB {
		hb = nb.MinHeadroom()
	}
	switch {
	case h != nil && *h < 0 && (hb == nil || *hb >= 0):
		out = append(out, fmt.Sprintf("`%s` is over capacity at peak (headroom %s)", id, pct(h)))
	case h != nil && *h >= 0 && *h < Tight && (hb == nil || *hb >= Tight):
		out = append(out, fmt.Sprintf("`%s` has less than %s headroom at peak (%s)", id, pct(ptr(Tight)), pct(h)))
	}
	return out
}

func ptr(v float64) *float64 { return &v }

func pathChanges(before, after []engine.PathResult) []PathChange {
	b, a := map[string]engine.PathResult{}, map[string]engine.PathResult{}
	for _, p := range before {
		b[strings.Join(p.Nodes, " → ")] = p
	}
	for _, p := range after {
		a[strings.Join(p.Nodes, " → ")] = p
	}
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

// DiffMarkdown writes a diff as a pull request comment.
func DiffMarkdown(w io.Writer, d Diff) error {
	b := &strings.Builder{}
	b.WriteString(Marker + "\n")
	fmt.Fprintf(b, "### archgopher · %s\n\n", orDash(d.Name))
	fmt.Fprintf(b, "Monthly: **%s → %s** (%s)\n\n", usd(d.BeforeUSD), usd(d.AfterUSD), delta(d.BeforeUSD, d.AfterUSD))
	if d.Empty() {
		b.WriteString("No change in cost, headroom or paths.\n")
		_, err := io.WriteString(w, b.String())
		return err
	}
	for _, a := range d.Alerts {
		fmt.Fprintf(b, "> [!WARNING]\n> %s\n\n", a)
	}
	if len(d.Nodes) > 0 {
		b.WriteString("| Node | | Monthly | Change | Tightest headroom |\n| --- | --- | ---: | ---: | ---: |\n")
		for _, n := range d.Nodes {
			fmt.Fprintf(b, "| `%s` %s | %s | %s | %s | %s |\n", n.ID, n.Label, n.Change,
				span(n.Change, usd(n.BeforeUSD), usd(n.AfterUSD)), delta(n.BeforeUSD, n.AfterUSD),
				span(n.Change, pct(n.BeforeHeadroom), pct(n.AfterHeadroom)))
		}
		b.WriteString("\n<details><summary>Cost lines</summary>\n\n| Node | Component | Before | After |\n| --- | --- | ---: | ---: |\n")
		for _, n := range d.Nodes {
			for _, l := range n.Lines {
				fmt.Fprintf(b, "| `%s` | %s | %s | %s |\n", n.ID, l.Name, lineUSD(l.Before), lineUSD(l.After))
			}
		}
		b.WriteString("\n</details>\n")
	}
	if len(d.Paths) > 0 {
		b.WriteString("\n<details><summary>Paths</summary>\n\n| Path | | p99 | Availability |\n| --- | --- | ---: | ---: |\n")
		for _, p := range d.Paths {
			fmt.Fprintf(b, "| %s | %s | %s | %s |\n", p.Path, p.Change, pathSpan(p, func(r engine.PathResult) string { return num(r.P99Ms) + " ms" }),
				pathSpan(p, func(r engine.PathResult) string { return availability(r.Availability) }))
		}
		b.WriteString("\n</details>\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func span(change, before, after string) string {
	switch change {
	case "added":
		return after
	case "removed":
		return before
	}
	if before == after {
		return after
	}
	return before + " → " + after
}

func pathSpan(p PathChange, f func(engine.PathResult) string) string {
	switch {
	case p.Before == nil:
		return f(*p.After)
	case p.After == nil:
		return f(*p.Before)
	}
	return span("changed", f(*p.Before), f(*p.After))
}

func lineUSD(v *float64) string {
	if v == nil {
		return "-"
	}
	return usd(*v)
}

// delta writes a signed change with its percentage when there is a base.
func delta(before, after float64) string {
	d := after - before
	if math.Abs(d) < 0.005 {
		return "±$0.00"
	}
	sign := "+"
	if d < 0 {
		sign = "−"
	}
	s := sign + "$" + strconv.FormatFloat(math.Abs(d), 'f', 2, 64)
	if before >= 0.005 {
		s += fmt.Sprintf(", %s%s%%", sign, strconv.FormatFloat(math.Abs(d)/before*100, 'f', 1, 64))
	}
	return s
}
