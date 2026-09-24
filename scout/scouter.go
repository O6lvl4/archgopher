package scout

import (
	"errors"
	"fmt"
	"math"
	"reflect"
)

// Common assumption keys every node accepts. The engine reads them itself.
const (
	LatencyP50 = "latencyP50Ms"
	LatencyP99 = "latencyP99Ms"
)

// LatencyFields are appended to every scouter's assumptions.
var LatencyFields = []Field{
	{Key: LatencyP50, Label: "Latency p50", Type: Number, Unit: "ms", Hint: "One round trip through this node"},
	{Key: LatencyP99, Label: "Latency p99", Type: Number, Unit: "ms", Hint: "Paths add p99s, so the path value is an upper bound"},
}

// Meta is what a catalog shows about a scouter.
type Meta struct {
	Type        string `json:"type"`
	Label       string `json:"label"`
	Category    string `json:"category"`
	Description string `json:"description"`
	// Kinds is the work the node accepts; the first is the default for edges.
	Kinds []string `json:"kinds"`
	// SLA is the ID in the SLA book; empty for nodes without an SLA (entries).
	SLA string `json:"sla,omitempty"`
	// External is true for nodes that are never Terraform resources (model APIs).
	External bool `json:"external,omitempty"`
}

// Scouter reads one resource type.
type Scouter interface {
	Meta() Meta
	Attributes() []Field
	Assumptions() []Field
	Scout(node Node, demand Demand, r *Recorder) error
}

// Def builds a Scouter from two tagged structs: A for Terraform attributes,
// P for assumptions. Run receives them decoded and validated.
type Def[A, P any] struct {
	Info Meta
	Run  func(a A, p P, d Demand, r *Recorder)
}

func (d Def[A, P]) Meta() Meta { return d.Info }

func (d Def[A, P]) Attributes() []Field { return FieldsOf(reflect.TypeFor[A]()) }

func (d Def[A, P]) Assumptions() []Field {
	return append(FieldsOf(reflect.TypeFor[P]()), LatencyFields...)
}

func (d Def[A, P]) Scout(node Node, demand Demand, r *Recorder) error {
	var a A
	var p P
	errA := Decode(node.Attributes, &a, "attribute")
	errP := Decode(node.Assumptions, &p, "assumption", LatencyP50, LatencyP99)
	if err := errors.Join(errA, errP); err != nil {
		return err
	}
	d.Run(a, p, demand, r)
	return r.err()
}

// Cost is one priced component: quantity × unit price.
type Cost struct {
	Name     string  `json:"name"`
	Quantity float64 `json:"quantity"`
	Unit     string  `json:"unit"`
	PriceID  string  `json:"priceId"`
	// UnitPrice and MonthlyUSD are nil when the price is not known.
	UnitPrice  *float64 `json:"unitPrice"`
	MonthlyUSD *float64 `json:"monthlyUsd"`
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
	// From says where the capacity came from: "quota" or "attribute".
	From string `json:"from"`
}

// RefUse records which reference values a node read.
type RefUse struct {
	Book     BookName `json:"book"`
	ID       string   `json:"id"`
	Region   string   `json:"region"`
	Verified bool     `json:"verified"`
	Known    bool     `json:"known"`
	Source   string   `json:"source"`
}

// Recorder collects readings while a scouter runs.
type Recorder struct {
	Region string
	books  Books
	costs  []Cost
	limits []Limit
	refs   []RefUse
	errs   []error
}

// NewRecorder returns a recorder for one node.
func NewRecorder(region string, books Books) *Recorder {
	return &Recorder{Region: region, books: books}
}

func (r *Recorder) lookup(book BookName, id, unit string) (Entry, *float64, bool) {
	e, v, err := r.books.Book(book).Lookup(id, r.Region)
	if err != nil {
		r.errs = append(r.errs, err)
		return Entry{}, nil, false
	}
	if e.Unit != unit {
		r.errs = append(r.errs, fmt.Errorf("%s %q is per %q but the reading counts %q", book, id, e.Unit, unit))
		return Entry{}, nil, false
	}
	r.refs = append(r.refs, RefUse{Book: book, ID: id, Region: r.Region, Verified: v.Verified, Known: v.Value != nil, Source: e.Source})
	return e, v.Value, true
}

// Cost records quantity (in unit) priced by priceID.
func (r *Recorder) Cost(name string, quantity float64, unit, priceID string) {
	e, v, ok := r.lookup(Prices, priceID, unit)
	if !ok {
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

// Limit records peak demand against the quota quotaID.
func (r *Recorder) Limit(name string, demand float64, unit, quotaID string) {
	e, v, ok := r.lookup(Quotas, quotaID, unit)
	if !ok {
		return
	}
	var capacity *float64
	if v != nil {
		c := e.PerUnit(*v)
		capacity = &c
	}
	r.limits = append(r.limits, newLimit(name, unit, demand, quotaID, capacity, "quota"))
}

// LimitScaled is Limit with the quota multiplied by factor (per-prefix limits × prefixes).
func (r *Recorder) LimitScaled(name string, demand float64, unit, quotaID string, factor float64) {
	e, v, ok := r.lookup(Quotas, quotaID, unit)
	if !ok {
		return
	}
	var capacity *float64
	if v != nil {
		c := e.PerUnit(*v) * factor
		capacity = &c
	}
	r.limits = append(r.limits, newLimit(name, unit, demand, quotaID, capacity, "quota"))
}

// Ref reads a raw reference value (a multiplier, a size cap). Nil means unknown.
func (r *Recorder) Ref(book BookName, id, unit string) *float64 {
	e, v, ok := r.lookup(book, id, unit)
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
	r.limits = append(r.limits, newLimit(name, unit, demand, "", override, "attribute"))
}

// Fail records a problem that makes the node's readings unreliable.
func (r *Recorder) Fail(format string, args ...any) {
	r.errs = append(r.errs, fmt.Errorf(format, args...))
}

func (r *Recorder) err() error { return errors.Join(r.errs...) }

func newLimit(name, unit string, demand float64, quotaID string, capacity *float64, from string) Limit {
	l := Limit{Name: name, Unit: unit, Demand: demand, QuotaID: quotaID, Capacity: capacity, From: from}
	if capacity != nil && *capacity > 0 {
		h := 1 - demand / *capacity
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
