package report

import (
	"strings"
	"testing"

	"github.com/O6lvl4/archgopher/engine"
	"github.com/O6lvl4/archgopher/meter"
)

func f(v float64) *float64 { return &v }

func node(id string, usd float64, headroom *float64, lines ...meter.Cost) engine.NodeResult {
	n := engine.NodeResult{ID: id, Label: "L", MonthlyUSD: usd, Costs: lines}
	if headroom != nil {
		n.Limits = []meter.Limit{{Name: "cap", Headroom: headroom}}
	}
	return n
}

func line(name string, usd float64) meter.Cost {
	return meter.Cost{Name: name, Unit: "u", MonthlyUSD: f(usd)}
}

// A diff lists nodes added, removed and read differently, biggest change
// first, leaves unchanged nodes out, and calls out a node that crosses into
// tight headroom or stops reading.
func TestCompare(t *testing.T) {
	before := engine.Result{Name: "app", MonthlyUSD: 30, Nodes: []engine.NodeResult{
		node("same", 10, f(0.9), line("Requests", 10)),
		node("grows", 15, f(0.5), line("Requests", 15)),
		node("gone", 5, nil, line("Storage", 5)),
	}, Paths: []engine.PathResult{{Nodes: []string{"users", "grows"}, P99Ms: 100, Availability: 0.999}}}
	after := engine.Result{Name: "app", MonthlyUSD: 70, Nodes: []engine.NodeResult{
		node("same", 10, f(0.9), line("Requests", 10)),
		node("grows", 58, f(0.1), line("Requests", 55), line("Duration", 3)),
		node("new", 2, nil, line("Storage", 2)),
		{ID: "broken", Label: "L", Error: "missing assumption \"x\""},
	}, Paths: []engine.PathResult{{Nodes: []string{"users", "grows"}, P99Ms: 250, Availability: 0.999}}}

	d := Compare(before, after)
	var got []string
	for _, n := range d.Nodes {
		got = append(got, n.Change+" "+n.ID)
	}
	if want := "changed grows, removed gone, added new, added broken"; strings.Join(got, ", ") != want {
		t.Fatalf("nodes: %v\nwant %s", got, want)
	}
	if lines := d.Nodes[0].Lines; len(lines) != 2 || lines[0].Name != "Requests" || lines[1].Before != nil {
		t.Fatalf("lines: %+v", lines)
	}
	if len(d.Paths) != 1 || d.Paths[0].Change != "changed" {
		t.Fatalf("paths: %+v", d.Paths)
	}
	alerts := strings.Join(d.Alerts, "\n")
	if !strings.Contains(alerts, "`grows` has less than 20.0% headroom") || !strings.Contains(alerts, "`broken` no longer reads") {
		t.Fatalf("alerts:\n%s", alerts)
	}

	wantDiffMarkdown(t, d, Marker, "**$30.00 → $70.00** (+$40.00, +133.3%)", "| `grows` L | changed | $15.00 → $58.00 | +$43.00, +286.7% | 50.0% → 10.0% |", "| `gone` L | removed | $5.00 | −$5.00, −100.0% |")
}

// wantDiffMarkdown fails unless the diff's comment contains every wanted text.
func wantDiffMarkdown(t *testing.T, d Diff, wants ...string) {
	t.Helper()
	var b strings.Builder
	if err := DiffMarkdown(&b, d); err != nil {
		t.Fatal(err)
	}
	md := b.String()
	for _, want := range wants {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q:\n%s", want, md)
		}
	}
}

func TestCompareNothing(t *testing.T) {
	r := engine.Result{Name: "app", MonthlyUSD: 1, Nodes: []engine.NodeResult{node("a", 1, f(0.5), line("x", 1))}}
	d := Compare(r, r)
	if !d.Empty() {
		t.Fatalf("%+v", d)
	}
}
