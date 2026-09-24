package scout

import (
	"math"
	"strings"
	"testing"
)

type pipeAttrs struct{}
type pipeAssume struct {
	Seconds float64 `scout:"seconds"`
}

// pipe is a test scouter: requests priced per million, concurrency against a quota.
var pipe = Def[pipeAttrs, pipeAssume]{
	Info: Meta{Type: "pipe", Label: "Pipe", Kinds: []string{"read", "write"}, SLA: "pipe"},
	Run: func(_ pipeAttrs, p pipeAssume, d Demand, r *Recorder) {
		r.Cost("Reads", d.Of("read").Monthly, "request", "pipe.requests")
		r.Cost("Writes", d.Of("write").Monthly, "request", "pipe.requests")
		r.Limit("Concurrency", d.Total().PeakPerSecond*p.Seconds, "concurrent", "pipe.concurrency")
	},
}

var badUnit = Def[pipeAttrs, pipeAssume]{
	Info: Meta{Type: "bad", Label: "Bad", Kinds: []string{"x"}},
	Run: func(_ pipeAttrs, _ pipeAssume, d Demand, r *Recorder) {
		r.Cost("Oops", 1, "GB", "pipe.requests")
	},
}

func books() Books {
	return Books{
		Prices: Book{"pipe.requests": {Unit: "request", Per: 1e6, Source: "test", Values: map[string]Value{"r1": {Value: f(2), Verified: true}}}},
		Quotas: Book{"pipe.concurrency": {Unit: "concurrent", Source: "test", Values: map[string]Value{AnyRegion: {Value: f(100)}}}},
		SLAs:   Book{"pipe": {Unit: "fraction", Source: "test", Values: map[string]Value{AnyRegion: {Value: f(0.99), Verified: true}}}},
	}
}

func registry() Registry {
	reg := Registry{}
	reg.Register(EntryScouter, pipe, badUnit)
	return reg
}

func node(id string, seconds float64) Node {
	return Node{ID: id, Type: "pipe", Assumptions: map[string]any{"seconds": seconds, LatencyP50: 10, LatencyP99: 50}}
}

func TestLoadFlowsByKindAndFactor(t *testing.T) {
	spec := Spec{Region: "r1",
		Nodes: []Node{{ID: "u", Type: EntryType, Load: &Load{Monthly: 1e6, PeakPerSecond: 10}}, node("a", 1), node("b", 2)},
		Edges: []Edge{{From: "u", To: "a"}, {From: "a", To: "b", Kind: "write", PerUnit: f(3)}, {From: "a", To: "b", Kind: "read"}},
	}
	res, err := Run(spec, registry(), books())
	if err != nil {
		t.Fatal(err)
	}
	b := res.Nodes[2]
	if b.Demand["write"].Monthly != 3e6 || b.Demand["read"].Monthly != 1e6 || b.Demand["write"].PeakPerSecond != 30 {
		t.Fatalf("demand %+v", b.Demand)
	}
	// a: 1M reads ($2); b: 1M reads + 3M writes ($8)
	if math.Abs(res.MonthlyUSD-10) > 1e-9 {
		t.Fatalf("total %v", res.MonthlyUSD)
	}
	if h := *b.Limits[0].Headroom; math.Abs(h-(1-40*2.0/100)) > 1e-9 {
		t.Fatalf("headroom %v", h)
	}
	if len(res.Paths) != 1 || res.Paths[0].P99Ms != 100 || math.Abs(res.Paths[0].Availability-0.99*0.99) > 1e-12 {
		t.Fatalf("paths %+v", res.Paths)
	}
	if len(res.Unverified) != 1 || res.Unverified[0].ID != "pipe.concurrency" {
		t.Fatalf("unverified %+v", res.Unverified)
	}
}

func TestNodeErrorsDoNotStopTheFlow(t *testing.T) {
	spec := Spec{Region: "r1",
		Nodes: []Node{{ID: "u", Type: EntryType, Load: &Load{Monthly: 1e6, PeakPerSecond: 1}}, {ID: "a", Type: "pipe"}, node("b", 1), {ID: "c", Type: "unknown"}},
		Edges: []Edge{{From: "u", To: "a"}, {From: "a", To: "b"}, {From: "b", To: "c"}},
	}
	res, err := Run(spec, registry(), books())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Nodes[1].Error, `missing assumption "seconds"`) {
		t.Fatalf("want a missing assumption, got %q", res.Nodes[1].Error)
	}
	if res.Nodes[2].Demand.Total().Monthly != 1e6 {
		t.Fatal("load must pass a node in error")
	}
	if res.Nodes[3].Skipped == "" || res.Nodes[3].Demand.Total().Monthly != 1e6 {
		t.Fatal("an unknown type is skipped but still receives load")
	}
}

func TestStructuralErrors(t *testing.T) {
	cases := map[string]Spec{
		"cycle":     {Region: "r1", Nodes: []Node{node("a", 1), node("b", 1)}, Edges: []Edge{{From: "a", To: "b"}, {From: "b", To: "a"}}},
		"dangling":  {Region: "r1", Nodes: []Node{node("a", 1)}, Edges: []Edge{{From: "a", To: "z"}}},
		"duplicate": {Region: "r1", Nodes: []Node{node("a", 1), node("a", 1)}},
		"kind":      {Region: "r1", Nodes: []Node{node("a", 1), node("b", 1)}, Edges: []Edge{{From: "a", To: "b", Kind: "delete"}}},
	}
	for name, spec := range cases {
		if _, err := Run(spec, registry(), books()); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestUnitMismatchIsAnError(t *testing.T) {
	spec := Spec{Region: "r1", Nodes: []Node{{ID: "x", Type: "bad", Assumptions: map[string]any{"seconds": 1}}}}
	res, err := Run(spec, registry(), books())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Nodes[0].Error, `per "request" but the reading counts "GB"`) {
		t.Fatalf("got %q", res.Nodes[0].Error)
	}
}

func TestEntryNeedsLoad(t *testing.T) {
	res, err := Run(Spec{Region: "r1", Nodes: []Node{{ID: "u", Type: EntryType}}}, registry(), books())
	if err != nil {
		t.Fatal(err)
	}
	if res.Nodes[0].Error == "" {
		t.Fatal("an entry without load must be an error")
	}
}

func TestSpecRoundTrip(t *testing.T) {
	in := Spec{Name: "n", Region: "r1", Nodes: []Node{{ID: "u", Type: EntryType, Load: &Load{Monthly: 3e7, PeakPerSecond: 0.25}}}}
	data, err := MarshalSpec(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "monthly: 30000000\n") {
		t.Fatalf("numbers should be plain decimals:\n%s", data)
	}
	out, err := ParseSpec(data)
	if err != nil || out.Nodes[0].Load.Monthly != 3e7 {
		t.Fatalf("round trip: %v %+v", err, out)
	}
	if _, err := ParseSpec([]byte("region: r1\nnodes: []\nedgez: []\n")); err == nil {
		t.Fatal("unknown keys must be rejected")
	}
}
