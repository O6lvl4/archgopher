package engine

import (
	"math"
	"testing"

	"github.com/O6lvl4/archgopher/book"
	"github.com/O6lvl4/archgopher/meter"
	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/scouter"
)

type boxAttrs struct{}
type boxAssume struct{}

// box is a server: a monthly price per box, and a peak each box can serve.
var box = scouter.Def[boxAttrs, boxAssume]{
	Info: scouter.Meta{Type: "box", Label: "Box", Kinds: []string{"request"}},
	Run: func(_ boxAttrs, _ boxAssume, d model.Demand, r *meter.Recorder) {
		r.Cost("Server", 1, "server-month", "box.server")
		r.LimitOverride("Requests per second", d.Total().PeakPerSecond, "requests/second", "", f(100))
	},
}

func instanceBooks(t *testing.T) book.Books {
	t.Helper()
	b := poolBooks(t)
	b.Prices["box.server"] = book.Entry{Unit: "server-month", Source: "test", Values: map[string]book.Value{"r1": {Value: f(10), Verified: true}}}
	return b
}

// readInstances reads a web box ×4, a fee ×3 and a box whose count is
// unknown before apply.
func readInstances(t *testing.T) map[string]NodeResult {
	t.Helper()
	reg := scouter.Registry{}
	reg.Register(box, fee)
	spec := model.Spec{Region: "r1", Nodes: []model.Node{
		{ID: "web", Type: "box", Instances: 4, Load: &model.Load{Monthly: 1e6, PeakPerSecond: 200}},
		{ID: "api", Type: "fee", Instances: 3, Load: &model.Load{Monthly: 6e6}},
		{ID: "new", Type: "box", Instances: model.UnknownInstances},
	}}
	res, err := Run(spec, reg, instanceBooks(t))
	if err != nil {
		t.Fatal(err)
	}
	return resultsByID(res.Nodes)
}

func TestInstancesMultiplyTheCostAndEachTakesAShare(t *testing.T) {
	web := readInstances(t)["web"]
	// Four servers at $10; each takes a quarter of the 200 a second peak.
	if web.Instances != 4 || web.Costs[0].Quantity != 4 || *web.Costs[0].MonthlyUSD != 40 {
		t.Errorf("web = %d instances, %+v", web.Instances, web.Costs[0])
	}
	if l := web.Limits[0]; l.Demand != 50 || *l.Headroom != 0.5 {
		t.Errorf("each server's peak = %v, headroom %v; want 50, 0.5", l.Demand, *l.Headroom)
	}
}

func TestInstancesKeepLoadCostsAndFeesPaidOnce(t *testing.T) {
	api := readInstances(t)["api"]
	// The three read their thirds, and the requests add back up to the node's
	// 6M: 1M included, 5M × $10/M. The regional fee is paid once, not three times.
	if api.Costs[0].Quantity != 6e6 || math.Abs(*api.Costs[0].MonthlyUSD-50) > 1e-9 {
		t.Errorf("api requests = %+v", api.Costs[0])
	}
	if api.Costs[1].Quantity != 730 {
		t.Errorf("the regional fee counts %v hours, want 730 once", api.Costs[1].Quantity)
	}
}

func TestUnknownInstancesReadOne(t *testing.T) {
	if n := readInstances(t)["new"]; n.Instances != 0 || *n.Costs[0].MonthlyUSD != 10 {
		t.Errorf("unknown count = %d instances, %+v", n.Instances, n.Costs[0])
	}
	reg := scouter.Registry{}
	reg.Register(box)
	if _, err := Run(model.Spec{Region: "r1", Nodes: []model.Node{{ID: "x", Type: "box", Instances: -2}}}, reg, instanceBooks(t)); err == nil {
		t.Error("instances -2 should be refused")
	}
}
