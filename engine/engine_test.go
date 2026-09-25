package engine

import (
	"math"
	"strings"
	"testing"

	"github.com/O6lvl4/archgopher/book"
	"github.com/O6lvl4/archgopher/meter"
	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/scouter"
)

type pipeAttrs struct{}
type pipeAssume struct {
	Seconds float64 `scout:"seconds"`
}

// pipe is a test scouter: requests priced per million, concurrency against a quota.
var pipe = scouter.Def[pipeAttrs, pipeAssume]{
	Info: scouter.Meta{Type: "pipe", Label: "Pipe", Kinds: []string{"read", "write"}, SLA: "pipe"},
	Run: func(_ pipeAttrs, p pipeAssume, d model.Demand, r *meter.Recorder) {
		r.Cost("Reads", d.Of("read").Monthly, "request", "pipe.requests")
		r.Cost("Writes", d.Of("write").Monthly, "request", "pipe.requests")
		r.Limit("Concurrency", d.Total().PeakPerSecond*p.Seconds, "concurrent", "pipe.concurrency")
	},
}

var badUnit = scouter.Def[pipeAttrs, pipeAssume]{
	Info: scouter.Meta{Type: "bad", Label: "Bad", Kinds: []string{"x"}},
	Run: func(_ pipeAttrs, _ pipeAssume, d model.Demand, r *meter.Recorder) {
		r.Cost("Oops", 1, "GB", "pipe.requests")
	},
}

func books() book.Books {
	return book.Books{
		Prices: book.Book{"pipe.requests": {Unit: "request", Per: 1e6, Source: "test", Values: map[string]book.Value{"r1": {Value: f(2), Verified: true}}}},
		Quotas: book.Book{"pipe.concurrency": {Unit: "concurrent", Source: "test", Values: map[string]book.Value{book.AnyRegion: {Value: f(100)}}}},
		SLAs:   book.Book{"pipe": {Unit: "fraction", Source: "test", Values: map[string]book.Value{book.AnyRegion: {Value: f(0.99), Verified: true}}}},
	}
}

func registry() scouter.Registry {
	reg := scouter.Registry{}
	reg.Register(scouter.EntryScouter, pipe, badUnit)
	return reg
}

func node(id string, seconds float64) model.Node {
	return model.Node{ID: id, Type: "pipe", Assumptions: map[string]any{"seconds": seconds, scouter.LatencyP50: 10, scouter.LatencyP99: 50}}
}

func TestLoadFlowsByKindAndFactor(t *testing.T) {
	spec := model.Spec{Region: "r1",
		Nodes: []model.Node{{ID: "u", Type: scouter.EntryType, Load: &model.Load{Monthly: 1e6, PeakPerSecond: 10}}, node("a", 1), node("b", 2)},
		Edges: []model.Edge{{From: "u", To: "a"}, {From: "a", To: "b", Kind: "write", PerUnit: f(3)}, {From: "a", To: "b", Kind: "read"}},
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
	spec := model.Spec{Region: "r1",
		Nodes: []model.Node{{ID: "u", Type: scouter.EntryType, Load: &model.Load{Monthly: 1e6, PeakPerSecond: 1}}, {ID: "a", Type: "pipe"}, node("b", 1), {ID: "c", Type: "unknown"}},
		Edges: []model.Edge{{From: "u", To: "a"}, {From: "a", To: "b"}, {From: "b", To: "c"}},
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
	cases := map[string]model.Spec{
		"cycle":      {Region: "r1", Nodes: []model.Node{node("a", 1), node("b", 1)}, Edges: []model.Edge{{From: "a", To: "b"}, {From: "b", To: "a"}}},
		"dangling":   {Region: "r1", Nodes: []model.Node{node("a", 1)}, Edges: []model.Edge{{From: "a", To: "z"}}},
		"duplicate":  {Region: "r1", Nodes: []model.Node{node("a", 1), node("a", 1)}},
		"kind":       {Region: "r1", Nodes: []model.Node{node("a", 1), node("b", 1)}, Edges: []model.Edge{{From: "a", To: "b", Kind: "delete"}}},
		"no group":   {Region: "r1", Nodes: []model.Node{grouped("a", "vpc")}},
		"two groups": {Region: "r1", Groups: []model.Group{{ID: "vpc", Kind: "VPC"}, {ID: "vpc", Kind: "VPC"}}},
		"ops and a kind": {Region: "r1", Nodes: []model.Node{node("a", 1), node("b", 1)},
			Edges: []model.Edge{{From: "a", To: "b", Kind: "read", Ops: []model.Op{{Kind: "write"}}}}},
		"load and traffic": {Region: "r1", Nodes: []model.Node{{ID: "a", Type: "entry", Load: &model.Load{Monthly: 1},
			Traffic: &model.Traffic{Schedule: "rate(1 hour)"}}}},
	}
	for name, spec := range cases {
		if _, err := Run(spec, registry(), books()); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestUnitMismatchIsAnError(t *testing.T) {
	spec := model.Spec{Region: "r1", Nodes: []model.Node{{ID: "x", Type: "bad", Assumptions: map[string]any{"seconds": 1}}}}
	res, err := Run(spec, registry(), books())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Nodes[0].Error, `per "request" but the reading counts "GB"`) {
		t.Fatalf("got %q", res.Nodes[0].Error)
	}
}

func TestEntryNeedsLoad(t *testing.T) {
	res, err := Run(model.Spec{Region: "r1", Nodes: []model.Node{{ID: "u", Type: scouter.EntryType}}}, registry(), books())
	if err != nil {
		t.Fatal(err)
	}
	if res.Nodes[0].Error == "" {
		t.Fatal("an entry without load must be an error")
	}
}

func grouped(id, group string) model.Node {
	n := node(id, 1)
	n.Group = group
	return n
}

// Groups are drawn, not read: a node in a group reads the same as without one.
func TestGroupsDoNotChangeReadings(t *testing.T) {
	plain := model.Spec{Region: "r1", Nodes: []model.Node{node("a", 1)}}
	boxed := model.Spec{Region: "r1", Nodes: []model.Node{grouped("a", "vpc")}, Groups: []model.Group{{ID: "vpc", Kind: "VPC"}}}
	a, err := Run(plain, registry(), books())
	if err != nil {
		t.Fatal(err)
	}
	b, err := Run(boxed, registry(), books())
	if err != nil {
		t.Fatal(err)
	}
	if a.MonthlyUSD != b.MonthlyUSD {
		t.Errorf("monthly %v with a group, %v without", b.MonthlyUSD, a.MonthlyUSD)
	}
}

// Traffic becomes the node's load, with the arithmetic; traffic that cannot be
// read is an error on that node only.
func TestTrafficBecomesLoad(t *testing.T) {
	spec := model.Spec{Region: "r1", Nodes: []model.Node{
		{ID: "hourly", Type: "entry", Traffic: &model.Traffic{Schedule: "rate(1 hour)"}},
		{ID: "broken", Type: "entry", Traffic: &model.Traffic{Rate: &model.Rate{Count: 1, Per: "fortnight"}}},
	}}
	res, err := Run(spec, registry(), books())
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range res.Nodes {
		switch n.ID {
		case "hourly":
			if n.Load == nil || n.Load.Monthly != 730 || n.LoadBasis == "" || n.Error != "" {
				t.Errorf("hourly: %+v", n)
			}
		case "broken":
			if n.Load != nil || !strings.Contains(n.Error, "traffic:") {
				t.Errorf("broken: %+v", n)
			}
		}
	}
}
