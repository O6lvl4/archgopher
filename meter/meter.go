// Package meter is the L1 layer: the smallest readings a scouter can make.
// A cost is a quantity times a price looked up by id; a limit is a peak demand
// against a quota looked up by id. Units are checked against the book, so a
// reading counted in GB can never be priced per GB-month.
package meter

import (
	"errors"
	"fmt"
	"math"

	"github.com/O6lvl4/archgopher/book"
)

// Cost is one priced component: quantity × unit price.
type Cost struct {
	Name     string  `json:"name"`
	Quantity float64 `json:"quantity"`
	Unit     string  `json:"unit"`
	PriceID  string  `json:"priceId"`
	// UnitPrice and MonthlyUSD are nil when the price is not known. When the
	// price has tiers or included units, UnitPrice is what the line pays per
	// unit on average.
	UnitPrice  *float64 `json:"unitPrice"`
	MonthlyUSD *float64 `json:"monthlyUsd"`
	// Bands break a line priced by tiers or included units down by price. A
	// pooled line has none: its pool carries them.
	Bands []Band `json:"bands,omitempty"`
	// Pool is set when the price is billed on a whole account's usage: the
	// engine bills the pool once and shares it out by quantity. Until then,
	// and when the node is read alone, the line is billed as if it were the
	// only user of the pool.
	Pool string `json:"pool,omitempty"`
}

// Limit compares peak demand with a capacity.
type Limit struct {
	Name    string  `json:"name"`
	Unit    string  `json:"unit"`
	Demand  float64 `json:"demand"`
	QuotaID string  `json:"quotaId,omitempty"`
	// Capacity is nil when unknown. Headroom is 1 − demand/capacity; negative means over.
	Capacity *float64 `json:"capacity"`
	Headroom *float64 `json:"headroom"`
	// From says where the capacity came from: "quota" (the published
	// default), "account" (the value applied to the account) or "attribute".
	From string `json:"from"`
}

// RefUse records which reference values a node read.
type RefUse struct {
	Book     book.Name `json:"book"`
	ID       string    `json:"id"`
	Region   string    `json:"region"`
	Verified bool      `json:"verified"`
	Known    bool      `json:"known"`
	Source   string    `json:"source"`
}

// Recorder collects readings while a scouter runs.
type Recorder struct {
	Region string
	books  book.Books
	costs  []Cost
	limits []Limit
	refs   []RefUse
	errs   []error
}

// NewRecorder returns a recorder for one node.
func NewRecorder(region string, books book.Books) *Recorder {
	return &Recorder{Region: region, books: books}
}

func (r *Recorder) lookup(name book.Name, id, unit string) (book.Entry, *float64, bool) {
	e, v, err := r.books.Book(name).Lookup(id, r.Region)
	if err != nil {
		r.errs = append(r.errs, err)
		return book.Entry{}, nil, false
	}
	if e.Unit != unit {
		r.errs = append(r.errs, fmt.Errorf("%s %q is per %q but the reading counts %q", name, id, e.Unit, unit))
		return book.Entry{}, nil, false
	}
	r.refs = append(r.refs, RefUse{Book: name, ID: id, Region: r.Region, Verified: v.Verified, Known: v.Value != nil, Source: e.Source})
	return e, v.Value, true
}

// NotOfferedError is a reading whose price is verified to have no value in the
// region: the Price List does not offer it there.
type NotOfferedError struct{ Reading, Region, PriceID string }

func (e *NotOfferedError) Error() string {
	return fmt.Sprintf("%s is not offered in %s (no %q price)", e.Reading, e.Region, e.PriceID)
}

func (r *Recorder) verified(name book.Name, id string) bool {
	_, v, err := r.books.Book(name).Lookup(id, r.Region)
	return err == nil && v.Verified
}

// Cost records quantity (in unit) priced by priceID.
func (r *Recorder) Cost(name string, quantity float64, unit, priceID string) {
	if e, ok := r.books.Prices[priceID]; ok && e.Billed() {
		r.billed(name, quantity, unit, priceID, e)
		return
	}
	e, v, ok := r.lookup(book.Prices, priceID, unit)
	if !ok {
		return
	}
	if v == nil && r.verified(book.Prices, priceID) {
		r.errs = append(r.errs, &NotOfferedError{Reading: name, Region: r.Region, PriceID: priceID})
		return
	}
	c := Cost{Name: name, Quantity: quantity, Unit: unit, PriceID: priceID}
	if v != nil {
		p := e.PerUnit(*v)
		m := quantity * p
		c.UnitPrice, c.MonthlyUSD = &p, &m
	}
	r.costs = append(r.costs, c)
}

// billed records a line whose price has tiers, included units or a pool. A
// pooled line is billed with the recorder's other lines of its pool when
// they are read out: as if the node were the account's only user.
func (r *Recorder) billed(name string, quantity float64, unit, priceID string, e book.Entry) {
	if e.Unit != unit {
		r.errs = append(r.errs, fmt.Errorf("prices %q is per %q but the reading counts %q", priceID, e.Unit, unit))
		return
	}
	included := 0.0
	if e.Pool == "" {
		included = e.Included // a pool gives its units once, when shared
	}
	bill, err := Bill(r.books.Prices, priceID, r.Region, quantity, included)
	if err != nil {
		var no *NotOfferedError
		if errors.As(err, &no) {
			no.Reading = name
		}
		r.errs = append(r.errs, err)
		return
	}
	r.refs = append(r.refs, RefUse{Book: book.Prices, ID: priceID, Region: r.Region, Verified: bill.Verified, Known: bill.USD != nil, Source: e.Source})
	c := Cost{Name: name, Quantity: quantity, Unit: unit, PriceID: priceID}
	c.setBill(bill)
	if e.Pool != "" {
		c.Pool = PoolKey(e, priceID, r.Region)
	}
	r.costs = append(r.costs, c)
}

// setBill sets the line's price from a bill of its whole quantity.
func (c *Cost) setBill(b Billing) {
	c.Bands = b.Bands
	c.UnitPrice, c.MonthlyUSD = nil, nil
	if b.USD == nil {
		return
	}
	m := *b.USD
	c.MonthlyUSD = &m
	if c.Quantity > 0 {
		p := m / c.Quantity
		c.UnitPrice = &p
	}
}

// PoolKey names the pool a price's readings share: the price, and the
// region for a pool per region.
func PoolKey(e book.Entry, priceID, region string) string {
	if e.Pool == book.PoolRegion {
		return priceID + "@" + region
	}
	return priceID
}

// Limit records peak demand against the quota quotaID.
func (r *Recorder) Limit(name string, demand float64, unit, quotaID string) {
	e, v, ok := r.lookup(book.Quotas, quotaID, unit)
	if !ok {
		return
	}
	var capacity *float64
	if v != nil {
		c := e.PerUnit(*v)
		capacity = &c
	}
	r.limits = append(r.limits, withHeadroom(Limit{Name: name, Unit: unit, Demand: demand, QuotaID: quotaID, Capacity: capacity, From: quotaFrom(e)}))
}

// LimitScaled is Limit with the quota multiplied by factor (per-prefix limits × prefixes).
func (r *Recorder) LimitScaled(name string, demand float64, unit, quotaID string, factor float64) {
	e, v, ok := r.lookup(book.Quotas, quotaID, unit)
	if !ok {
		return
	}
	var capacity *float64
	if v != nil {
		c := e.PerUnit(*v) * factor
		capacity = &c
	}
	r.limits = append(r.limits, withHeadroom(Limit{Name: name, Unit: unit, Demand: demand, QuotaID: quotaID, Capacity: capacity, From: quotaFrom(e)}))
}

// Ref reads a raw reference value (a multiplier, a size cap). Nil means unknown.
func (r *Recorder) Ref(name book.Name, id, unit string) *float64 {
	e, v, ok := r.lookup(name, id, unit)
	if !ok || v == nil {
		return nil
	}
	x := e.PerUnit(*v)
	return &x
}

// LimitOverride is Limit where the resource itself sets the capacity
// (reserved concurrency, max ACU). A nil override falls back to the quota.
func (r *Recorder) LimitOverride(name string, demand float64, unit, quotaID string, override *float64) {
	if override == nil {
		r.Limit(name, demand, unit, quotaID)
		return
	}
	r.limits = append(r.limits, withHeadroom(Limit{Name: name, Unit: unit, Demand: demand, Capacity: override, From: "attribute"}))
}

// Fail records a problem that makes the node's readings unreliable.
func (r *Recorder) Fail(format string, args ...any) {
	r.errs = append(r.errs, fmt.Errorf(format, args...))
}

// Err joins every problem recorded so far.
func (r *Recorder) Err() error { return errors.Join(r.errs...) }

// Costs returns the cost lines recorded so far, the pooled ones billed
// together as the node's own pools. A pool that cannot be billed leaves its
// lines unpriced; each line was already checked on its own.
func (r *Recorder) Costs() []Cost {
	out := append([]Cost(nil), r.costs...)
	lines := make([]Owned, len(out))
	for i := range out {
		lines[i] = Owned{Cost: &out[i]}
	}
	Share(lines, r.books.Prices, r.Region)
	return out
}

// Limits returns the limits recorded so far.
func (r *Recorder) Limits() []Limit { return r.limits }

// Refs returns every reference value the recorder read.
func (r *Recorder) Refs() []RefUse { return r.refs }

// withHeadroom sets the share of a limit's capacity left at its demand; it
// stays unknown without a capacity above zero.
func withHeadroom(l Limit) Limit {
	if l.Capacity != nil && *l.Capacity > 0 {
		h := 1 - l.Demand / *l.Capacity
		l.Headroom = &h
	}
	return l
}

// CeilDiv rounds a/b up; billing units (4 KB reads, 64 KB messages) use it.
func CeilDiv(a, b float64) float64 {
	if a <= 0 {
		return 1
	}
	return math.Ceil(a / b)
}

// quotaFrom says whether a quota's value is the published default or the
// account's own.
func quotaFrom(e book.Entry) string {
	if e.Applied != "" {
		return "account"
	}
	return "quota"
}
