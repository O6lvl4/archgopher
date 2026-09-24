// Package facet is the L2 layer: reusable readings that scouters compose.
// A facet knows a billing or capacity shape (per request in size chunks,
// GB-seconds, storage, Little's law concurrency, tokens) but no provider:
// the service that uses it names the price and quota ids.
//
// Facets that need assumptions export a struct to embed in the scouter's
// assumption struct, so the fields, their validation and the UI form come
// with the facet.
package facet

import (
	"math"

	"github.com/O6lvl4/archgopher/meter"
	"github.com/O6lvl4/archgopher/model"
)

// Requests prices a count of requests, optionally billed in size chunks
// (64 KB messages, 512 KB API payloads).
type Requests struct {
	Name    string
	Unit    string
	PriceID string
	// ChunkKB > 0 bills each request once per started chunk.
	ChunkKB float64
}

// Read records count requests of sizeKB each.
func (f Requests) Read(r *meter.Recorder, count, sizeKB float64) {
	if f.ChunkKB > 0 {
		count *= meter.CeilDiv(sizeKB, f.ChunkKB)
	}
	r.Cost(f.Name, count, f.Unit, f.PriceID)
}

// Rate compares a peak rate with a quota.
type Rate struct {
	Name    string
	Unit    string
	QuotaID string
}

// Read records the peak against the quota.
func (f Rate) Read(r *meter.Recorder, peak float64) { r.Limit(f.Name, peak, f.Unit, f.QuotaID) }

// ReadScaled records the peak against the quota times factor (per-prefix limits × prefixes).
func (f Rate) ReadScaled(r *meter.Recorder, peak, factor float64) {
	r.LimitScaled(f.Name, peak, f.Unit, f.QuotaID, factor)
}

// Compute prices GB-seconds: count × seconds × memory.
type Compute struct {
	Name    string
	PriceID string
	// StepMB rounds memory up to a billing step (Step Functions express: 64 MB); 0 means exact.
	StepMB float64
}

// Read records the GB-seconds of count runs.
func (f Compute) Read(r *meter.Recorder, count, seconds, memoryMB float64) {
	if f.StepMB > 0 {
		memoryMB = meter.CeilDiv(memoryMB, f.StepMB) * f.StepMB
	}
	r.Cost(f.Name, count*seconds*memoryMB/1024, "GB-second", f.PriceID)
}

// Concurrency is Little's law: peak rate × duration against a quota, or
// against a capacity the resource sets itself (reserved concurrency).
type Concurrency struct {
	Name    string
	Unit    string
	QuotaID string
}

// Read records peak × seconds against the quota, or the override when set.
func (f Concurrency) Read(r *meter.Recorder, peak, seconds float64, override *float64) {
	r.LimitOverride(f.Name, peak*seconds, f.Unit, f.QuotaID, override)
}

// Storage prices an amount held for the month.
type Storage struct {
	Name    string
	PriceID string
}

// Read records amount GB held for the month.
func (f Storage) Read(r *meter.Recorder, gb float64) { r.Cost(f.Name, gb, "GB-month", f.PriceID) }

// Capacity prices something provisioned per hour for the whole month
// (read capacity units, Aurora capacity units).
type Capacity struct {
	Name    string
	Unit    string // e.g. "RCU-hour"
	PriceID string
}

// Read records units provisioned all month.
func (f Capacity) Read(r *meter.Recorder, units float64) {
	r.Cost(f.Name, units*model.HoursPerMonth, f.Unit, f.PriceID)
}

// round keeps derived quantities free of float noise in reports.
func round(v float64) float64 { return math.Round(v*1e9) / 1e9 }
