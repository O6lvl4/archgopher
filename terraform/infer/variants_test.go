package infer

import (
	"testing"

	"github.com/O6lvl4/archgopher/model"
)

func TestRewireSharesTheLoadIntoParts(t *testing.T) {
	half := 0.5
	edges := []model.Edge{
		{From: "lb", To: "app"},
		{From: "api", To: "app", Ops: []model.Op{{Kind: "read", PerUnit: &half}}},
		{From: "app", To: "db"},
	}
	parts := []model.Node{{ID: "app-web", Instances: 3}, {ID: "app-batch"}}
	got := rewire(edges, "app", parts)
	per := map[string]float64{}
	for _, e := range got {
		f := 1.0
		switch {
		case len(e.Ops) > 0:
			f = *e.Ops[0].PerUnit
		case e.PerUnit != nil:
			f = *e.PerUnit
		}
		per[e.From+">"+e.To] = f
	}
	// Three of four instances take three quarters of what comes in; every
	// part sends on what it takes.
	want := map[string]float64{
		"lb>app-web": 0.75, "lb>app-batch": 0.25,
		"api>app-web": 0.375, "api>app-batch": 0.125,
		"app-web>db": 1, "app-batch>db": 1,
	}
	if len(per) != len(want) {
		t.Fatalf("edges = %v", per)
	}
	for k, w := range want {
		if per[k] != w {
			t.Errorf("%s = %v, want %v", k, per[k], w)
		}
	}
}
