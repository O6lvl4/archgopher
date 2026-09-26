package meter

import (
	"fmt"
	"math"

	"github.com/O6lvl4/archgopher/book"
)

// Band is the part of a quantity one price applies to: the units a plan
// includes, or one volume tier.
type Band struct {
	From     float64 `json:"from"`
	To       float64 `json:"to,omitempty"` // 0 means no upper bound
	Quantity float64 `json:"quantity"`
	// UnitPrice is per single unit; nil when the tier's price is not known.
	UnitPrice *float64 `json:"unitPrice"`
	// Included marks units the plan includes.
	Included bool `json:"included,omitempty"`
}

// Billing is a quantity priced by an entry's rules.
type Billing struct {
	Bands []Band
	// USD is nil when a band with quantity has no known price.
	USD *float64
	// Verified is true when every price read was verified.
	Verified bool
}

// Bill prices quantity by the rules of the price id in region: the included
// units first, then each tier on the part of the quantity that falls in it.
// Tiers count from zero over the whole quantity, included units too. A
// zero-priced tier at the start is a free grant, which is not what the
// architecture costs month after month: it is billed at the first paid
// tier's price.
func Bill(prices book.Book, id, region string, quantity, included float64) (Billing, error) {
	e, ok := prices[id]
	if !ok {
		return Billing{}, fmt.Errorf("no reference entry %q", id)
	}
	tiers := e.Tiers
	if !e.Tiered {
		tiers = []book.Tier{{From: 0, ID: id}}
	}
	out := Billing{Verified: true}
	if included > 0 && quantity > 0 {
		out.Bands = append(out.Bands, Band{From: 0, To: included, Quantity: math.Min(quantity, included), UnitPrice: new(float64), Included: true})
	}
	s := tiering{prices: prices, region: region, tiers: tiers, grant: firstPaid(prices, tiers, region), quantity: quantity, included: included}
	for i := range tiers {
		b, verified, err := s.band(i)
		if err != nil {
			return Billing{}, err
		}
		out.Verified = out.Verified && verified
		if b.Quantity > 0 {
			out.Bands = append(out.Bands, b)
		}
	}
	out.USD = total(out.Bands)
	return out, nil
}

// total is what the bands cost; nil when one of them has no known price.
func total(bands []Band) *float64 {
	usd := 0.0
	for _, b := range bands {
		if b.UnitPrice == nil {
			return nil
		}
		usd += b.Quantity * *b.UnitPrice
	}
	return &usd
}

// tiering lays one quantity, less its included units, over the tiers of a price.
type tiering struct {
	prices             book.Book
	region             string
	tiers              []book.Tier
	grant              paid
	quantity, included float64
}

// band is the part of the quantity that falls in tier i, at the tier's unit
// price (the first paid tier's inside a free grant), and whether that price
// was verified. A verified price missing where there is quantity means the
// region does not offer it.
func (s tiering) band(i int) (Band, bool, error) {
	t := s.tiers[i]
	to := math.Inf(1)
	if i+1 < len(s.tiers) {
		to = s.tiers[i+1].From
	}
	lo := math.Max(t.From, s.included)
	q := math.Max(0, math.Min(s.quantity, to)-lo)
	te, v, err := s.prices.Lookup(t.ID, s.region)
	if err != nil {
		return Band{}, false, err
	}
	if v.Value == nil && v.Verified && q > 0 {
		return Band{}, false, &NotOfferedError{Region: s.region, PriceID: t.ID}
	}
	if i < s.grant.index {
		te, v.Value = s.grant.entry, s.grant.value
	}
	b := Band{From: lo, Quantity: q}
	if !math.IsInf(to, 1) {
		b.To = to
	}
	if v.Value != nil {
		p := te.PerUnit(*v.Value)
		b.UnitPrice = &p
	}
	return b, v.Verified, nil
}

// paid is the first tier with a price above zero, and its index; index 0
// when the first tier is paid or none is.
type paid struct {
	index int
	entry book.Entry
	value *float64
}

func firstPaid(prices book.Book, tiers []book.Tier, region string) paid {
	for i, t := range tiers {
		e, v, err := prices.Lookup(t.ID, region)
		if err != nil || v.Value == nil || *v.Value != 0 {
			if err == nil && v.Value != nil {
				return paid{index: i, entry: e, value: v.Value}
			}
			return paid{}
		}
	}
	return paid{}
}
