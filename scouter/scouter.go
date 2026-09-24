// Package scouter defines how one resource type is read: its catalog entry,
// the Terraform attributes and assumptions it accepts, and the function that
// turns the load it receives into meter readings.
package scouter

import (
	"errors"
	"reflect"
	"sort"

	"github.com/O6lvl4/archgopher/field"
	"github.com/O6lvl4/archgopher/meter"
	"github.com/O6lvl4/archgopher/model"
)

// Common assumption keys every node accepts. The engine reads them itself.
const (
	LatencyP50 = "latencyP50Ms"
	LatencyP99 = "latencyP99Ms"
)

// LatencyFields are appended to every scouter's assumptions.
var LatencyFields = []field.Field{
	{Key: LatencyP50, Label: "Latency p50", Type: field.Number, Unit: "ms", Hint: "One round trip through this node"},
	{Key: LatencyP99, Label: "Latency p99", Type: field.Number, Unit: "ms", Hint: "Paths add p99s, so the path value is an upper bound"},
}

// Meta is what a catalog shows about a scouter.
type Meta struct {
	Type     string `json:"type"`
	Label    string `json:"label"`
	Category string `json:"category"`
	// Provider is the cloud the resource belongs to ("aws", "azure"); empty
	// for provider-neutral nodes such as the entry.
	Provider    string `json:"provider,omitempty"`
	Description string `json:"description"`
	// Kinds is the work the node accepts; the first is the default for edges.
	Kinds []string `json:"kinds"`
	// SLA is the ID in the SLA book; empty for nodes without an SLA (entries).
	SLA string `json:"sla,omitempty"`
	// External is true for nodes that are never Terraform resources (model APIs).
	External bool `json:"external,omitempty"`
	// Boundary types are groups drawn around nodes (a VPC), never nodes.
	Boundary bool `json:"boundary,omitempty"`
	// Icon names the picture the UI draws for the node: "<provider>/<name>",
	// or "general/<name>" for provider-neutral nodes.
	Icon string `json:"icon,omitempty"`
}

// Scouter reads one resource type.
type Scouter interface {
	Meta() Meta
	Attributes() []field.Field
	Assumptions() []field.Field
	Scout(node model.Node, demand model.Demand, r *meter.Recorder) error
}

// Def builds a Scouter from two tagged structs: A for Terraform attributes,
// P for assumptions. Run receives them decoded and validated.
type Def[A, P any] struct {
	Info Meta
	Run  func(a A, p P, d model.Demand, r *meter.Recorder)
}

func (d Def[A, P]) Meta() Meta { return d.Info }

func (d Def[A, P]) Attributes() []field.Field { return field.FieldsOf(reflect.TypeFor[A]()) }

func (d Def[A, P]) Assumptions() []field.Field {
	return append(field.FieldsOf(reflect.TypeFor[P]()), LatencyFields...)
}

func (d Def[A, P]) Scout(node model.Node, demand model.Demand, r *meter.Recorder) error {
	var a A
	var p P
	errA := field.Decode(node.Attributes, &a, "attribute")
	errP := field.Decode(node.Assumptions, &p, "assumption", LatencyP50, LatencyP99)
	if err := errors.Join(errA, errP); err != nil {
		return err
	}
	d.Run(a, p, demand, r)
	return r.Err()
}

// Registry maps a node type to its scouter.
type Registry map[string]Scouter

// Register adds scouters, panicking on a duplicate type.
func (reg Registry) Register(ss ...Scouter) {
	for _, s := range ss {
		t := s.Meta().Type
		if _, dup := reg[t]; dup {
			panic("duplicate scouter " + t)
		}
		reg[t] = s
	}
}

// Types lists registered types in order.
func (reg Registry) Types() []string {
	out := make([]string, 0, len(reg))
	for t := range reg {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}
