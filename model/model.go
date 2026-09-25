// Package model holds the declaration: nodes, edges and the load that flows
// between them. It is the vocabulary every other package shares, and it
// depends on nothing but YAML.
package model

// HoursPerMonth follows the AWS convention for monthly estimates.
const HoursPerMonth = 730

// SecondsPerMonth is HoursPerMonth in seconds.
const SecondsPerMonth = HoursPerMonth * 3600

// Load is a pair: the monthly volume drives cost, the peak rate drives headroom.
type Load struct {
	Monthly       float64 `yaml:"monthly" json:"monthly"`
	PeakPerSecond float64 `yaml:"peakPerSecond" json:"peakPerSecond"`
}

// Add returns the sum of two loads.
func (l Load) Add(o Load) Load {
	return Load{Monthly: l.Monthly + o.Monthly, PeakPerSecond: l.PeakPerSecond + o.PeakPerSecond}
}

// Scale multiplies both halves of the load.
func (l Load) Scale(f float64) Load {
	return Load{Monthly: l.Monthly * f, PeakPerSecond: l.PeakPerSecond * f}
}

// Demand is the load a node receives, split by kind of work (read, write, invoke...).
type Demand map[string]Load

// Of returns the load of one kind, zero if absent.
func (d Demand) Of(kind string) Load { return d[kind] }

// Total is the node's throughput; downstream edges multiply this value.
func (d Demand) Total() Load {
	var t Load
	for _, l := range d {
		t = t.Add(l)
	}
	return t
}

// Spec is a declaration (*.scouter.yaml): nodes, edges and where they run.
type Spec struct {
	Name   string `yaml:"name" json:"name"`
	Region string `yaml:"region" json:"region"`
	Nodes  []Node `yaml:"nodes" json:"nodes"`
	Edges  []Edge `yaml:"edges" json:"edges"`
	// Groups are boundaries drawn around nodes, such as a VPC. The engine
	// reads nothing from them.
	Groups []Group `yaml:"groups,omitempty" json:"groups,omitempty"`
}

// Group is a boundary nodes sit in: a VPC, a virtual network.
type Group struct {
	ID string `yaml:"id" json:"id"`
	// Kind names the boundary for people ("VPC", "VNet").
	Kind  string `yaml:"kind" json:"kind"`
	Label string `yaml:"label,omitempty" json:"label,omitempty"`
	// Type selects the scouter that reads traffic between the group's nodes
	// (aws_vpc reads what crosses Availability Zones); empty reads nothing.
	Type string `yaml:"type,omitempty" json:"type,omitempty"`
	// Assumptions are the group's numbers, such as how many zones it spans.
	Assumptions map[string]any `yaml:"assumptions,omitempty" json:"assumptions,omitempty"`
	// Position and Size are where the UI draws the frame. The engine ignores them.
	Position *Position `yaml:"position,omitempty" json:"position,omitempty"`
	Size     *Size     `yaml:"size,omitempty" json:"size,omitempty"`
}

// Size is how big the UI draws a frame.
type Size struct {
	Width  float64 `yaml:"width" json:"width"`
	Height float64 `yaml:"height" json:"height"`
}

// Node is one resource (or one external dependency such as a model API).
type Node struct {
	ID string `yaml:"id" json:"id"`
	// Type selects the Scouter. For Terraform resources it is the resource type.
	Type string `yaml:"type" json:"type"`
	// Address is the Terraform address; empty for nodes outside Terraform.
	Address string `yaml:"address,omitempty" json:"address,omitempty"`
	// Attributes come from Terraform (snake_case keys).
	Attributes map[string]any `yaml:"attributes,omitempty" json:"attributes,omitempty"`
	// Assumptions are numbers Terraform cannot know (camelCase keys).
	Assumptions map[string]any `yaml:"assumptions,omitempty" json:"assumptions,omitempty"`
	// Load makes the node an entry point of the graph: volume and peak.
	Load *Load `yaml:"load,omitempty" json:"load,omitempty"`
	// Traffic is the same said another way (users and their actions, a
	// schedule, ...); it is turned into a Load. A node has one or the other.
	Traffic  *Traffic  `yaml:"traffic,omitempty" json:"traffic,omitempty"`
	Note     string    `yaml:"note,omitempty" json:"note,omitempty"`
	Position *Position `yaml:"position,omitempty" json:"position,omitempty"`
	// Group is the id of the boundary the node sits in; empty for none.
	Group string `yaml:"group,omitempty" json:"group,omitempty"`
	// Stale marks a node whose Terraform address disappeared on the last merge.
	Stale bool `yaml:"stale,omitempty" json:"stale,omitempty"`
}

// Position is where the UI draws the node. The engine ignores it.
type Position struct {
	X float64 `yaml:"x" json:"x"`
	Y float64 `yaml:"y" json:"y"`
}

// Edge sends load from one node to another.
type Edge struct {
	From string `yaml:"from" json:"from"`
	To   string `yaml:"to" json:"to"`
	// Kind is the work the downstream node receives; empty means its default.
	Kind string `yaml:"kind,omitempty" json:"kind,omitempty"`
	// PerUnit is how many downstream units one upstream unit causes; nil means 1.
	PerUnit *float64 `yaml:"perUnit,omitempty" json:"perUnit,omitempty"`
	// KB is the data one downstream unit moves over the edge, both ways. Between
	// two nodes of one group it is the traffic the group reads (zone crossings).
	KB   *float64 `yaml:"kb,omitempty" json:"kb,omitempty"`
	Note string   `yaml:"note,omitempty" json:"note,omitempty"`
}

// Factor returns PerUnit, defaulting to 1.
func (e Edge) Factor() float64 {
	if e.PerUnit == nil {
		return 1
	}
	return *e.PerUnit
}
