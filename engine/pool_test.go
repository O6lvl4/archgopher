package engine

import (
	"math"
	"testing"

	"github.com/O6lvl4/archgopher/book"
	"github.com/O6lvl4/archgopher/meter"
	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/scouter"
)

type feeAttrs struct{}
type feeAssume struct{}

// fee reads a regional fee paid once while any user needs it, and requests
// that share volume tiers and a plan's included units across the account.
var fee = scouter.Def[feeAttrs, feeAssume]{
	Info: scouter.Meta{Type: "fee", Label: "Fee", Kinds: []string{"request"}},
	Run: func(_ feeAttrs, _ feeAssume, d model.Demand, r *meter.Recorder) {
		r.Cost("Requests", d.Total().Monthly, "request", "fee.requests")
		r.Cost("Regional fee", 730, "hour", "fee.regional")
	},
}

func poolBooks(t *testing.T) book.Books {
	t.Helper()
	prices, err := book.Book{
		"fee.requests": {Unit: "request", Per: 1e6, Source: "test", Pool: book.PoolRegion, Included: 1e6, Tiered: true, Verified: true, Rows: map[string]map[string]*float64{
			"0": {"r1": f(10)}, "10000000": {"r1": f(5)},
		}},
		"fee.regional": {Unit: "hour", Source: "test", Pool: book.PoolRegion, Combine: book.CombineMax, Values: map[string]book.Value{"r1": {Value: f(2), Verified: true}}},
	}.Flatten()
	if err != nil {
		t.Fatal(err)
	}
	return book.Books{Prices: prices, Quotas: book.Book{}, SLAs: book.Book{}}
}

func poolSpec() model.Spec {
	return model.Spec{Region: "r1",
		Nodes: []model.Node{
			{ID: "a", Type: "fee", Load: &model.Load{Monthly: 6e6}},
			{ID: "b", Type: "fee", Load: &model.Load{Monthly: 9e6}},
		},
	}
}

func TestPoolsBillTheAccountOnce(t *testing.T) {
	reg := scouter.Registry{}
	reg.Register(fee)
	res, err := Run(poolSpec(), reg, poolBooks(t))
	if err != nil {
		t.Fatal(err)
	}
	// 15M requests: 1M included, 9M × $10/M, 5M × $5/M = $115, shared 6:9.
	// The fee is 730 h × $2 once, shared 1:1. Alone, a would pay $50 + $1460.
	byID := map[string]NodeResult{}
	for _, n := range res.Nodes {
		byID[n.ID] = n
	}
	if got := *byID["a"].Costs[0].MonthlyUSD; math.Abs(got-46) > 1e-9 {
		t.Errorf("a's requests = %v, want 46", got)
	}
	if got := *byID["b"].Costs[1].MonthlyUSD; math.Abs(got-730) > 1e-9 {
		t.Errorf("b's share of the fee = %v, want 730", got)
	}
	if math.Abs(res.MonthlyUSD-(115+1460)) > 1e-9 {
		t.Errorf("total = %v, want %v", res.MonthlyUSD, 115+1460)
	}
	if len(res.Pools) != 2 || res.Pools[0].Key != "fee.regional@r1" || res.Pools[0].Quantity != 730 || res.Pools[1].Quantity != 15e6 || len(res.Pools[1].Members) != 2 {
		t.Fatalf("pools = %+v", res.Pools)
	}
}
