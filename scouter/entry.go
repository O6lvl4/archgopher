package scouter

import (
	"github.com/O6lvl4/archgopher/field"
	"github.com/O6lvl4/archgopher/meter"
	"github.com/O6lvl4/archgopher/model"
)

// EntryType is the node type of a pure load source: users, clients, another system.
const EntryType = "entry"

type none struct{}

// EntryScouter is the scouter for entry nodes. It reads nothing; it only checks that
// a load is set, because an entry without load silently zeroes the graph.
var EntryScouter Scouter = entryScouter{}

type entryScouter struct{}

func (entryScouter) Meta() Meta {
	return Meta{
		Type: EntryType, Label: "Entry", Category: "Entry",
		Description: "Where load comes from: users, clients or another system. Set monthly volume and peak rate.",
		Kinds:       []string{"unit"}, External: true,
	}
}
func (entryScouter) Attributes() []field.Field  { return nil }
func (entryScouter) Assumptions() []field.Field { return nil }
func (entryScouter) Scout(n model.Node, _ model.Demand, r *meter.Recorder) error {
	if n.Load == nil {
		r.Fail("an entry needs load (monthly and peakPerSecond)")
	}
	if err := field.Decode(n.Assumptions, &none{}, "assumption", LatencyP50, LatencyP99); err != nil {
		r.Fail("%v", err)
	}
	return r.Err()
}
