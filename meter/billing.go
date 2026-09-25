package meter

import (
	"fmt"
	"math"

	"github.com/O6lvl4/archgopher/book"
)

// Band is the part of a quantity one price applies to: the free units, or
// one volume tier.
type Band struct {
	From     float64 `json:"from"`
	To       float64 `json:"to,omitempty"` // 0 means no upper bound
	Quantity float64 `json:"quantity"`
	// UnitPrice is per single unit; nil when the tier's price is not known.
	UnitPrice *float64 `json:"unitPrice"`
	Free      bool     `json:"free,omitempty"`
}

// Billing is a quantity priced by an entry's rules.
type Billing struct {
	Bands []Band
	// USD is nil when a band with quantity has no known price.
	USD *float64
	// Verified is true when every price read was verified.
	Verified bool
}

// Bill prices quantity by the rules of the price id in region: allowance
// units free first, then each tier on the part of the quantity that falls in
// it. Tiers count from zero over the whole quantity, free units included.
func Bill(prices book.Book, id, region string, quantity, allowance float64) (Billing, error) {
	e, ok := prices[id]
	if !ok {
		return Billing{}, fmt.Errorf("no reference entry %q", id)
	}
	tiers := e.Tiers
	if !e.Tiered {
		tiers = []book.Tier{{From: 0, ID: id}}
	}
	out := Billing{Verified: true}
	if allowance > 0 && quantity > 0 {
		out.Bands = append(out.Bands, Band{From: 0, To: allowance, Quantity: math.Min(quantity, allowance), UnitPrice: new(float64), Free: true})
	}
	total := 0.0
	known := true
	for i, t := range tiers {
		to := math.Inf(1)
		if i+1 < len(tiers) {
			to = tiers[i+1].From
		}
		lo := math.Max(t.From, allowance)
		q := math.Max(0, math.Min(quantity, to)-lo)
		te, v, err := prices.Lookup(t.ID, region)
		if err != nil {
			return Billing{}, err
		}
		out.Verified = out.Verified && v.Verified
		if v.Value == nil && v.Verified && q > 0 {
			return Billing{}, &NotOfferedError{Region: region, PriceID: t.ID}
		}
		b := Band{From: lo, Quantity: q}
		if !math.IsInf(to, 1) {
			b.To = to
		}
		if v.Value != nil {
			p := te.PerUnit(*v.Value)
			b.UnitPrice = &p
			total += q * p
		} else if q > 0 {
			known = false
		}
		if q > 0 {
			out.Bands = append(out.Bands, b)
		}
	}
	if known {
		out.USD = &total
	}
	return out, nil
}
