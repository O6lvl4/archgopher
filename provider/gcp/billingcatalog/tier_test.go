package billingcatalog

import "testing"

func TestPickTierCountsStartsInBookUnits(t *testing.T) {
	// Tiers of a SKU sold per month, read by a book counting hours (listPer 730).
	rates := []Rate{{Start: 0, UnitPrice: Money{Units: "0"}}, {Start: 6, UnitPrice: Money{Units: "1"}}, {Start: 100, UnitPrice: Money{Units: "2"}}}
	for tier, want := range map[string]float64{"0": 0, "4380": 6, "73000": 100, "5000": 6} {
		r, err := pickTier(rates, tier, 730)
		if err != nil || r.Start != want {
			t.Errorf("tier %s: got start %v (%v), want %v", tier, r.Start, err, want)
		}
	}
}

func TestPickTierInEffectWhereARegionDoesNotBreak(t *testing.T) {
	// Another region breaks at 1024; this one does not, so 1024 bills tier 0.
	rates := []Rate{{Start: 0, UnitPrice: Money{Units: "1"}}, {Start: 10240, UnitPrice: Money{Nanos: 500000000}}}
	r, err := pickTier(rates, "1024", 0)
	if err != nil || r.Start != 0 {
		t.Fatalf("got %+v, %v", r, err)
	}
}
