// Package model holds the declaration: nodes, edges and the load that flows
// between them. It is the vocabulary every other package shares, and it
// depends on nothing but YAML.
package model

// HoursPerMonth follows the AWS convention for monthly estimates.
const HoursPerMonth = 730

// SecondsPerMonth is HoursPerMonth in seconds.
const SecondsPerMonth = HoursPerMonth * 3600

// Load is a pair: the monthly volume drives cost, the peak rate drives
// headroom. Sizes says how much of it came in operations of a known size;
// the rest is of the size the receiving node assumes.
type Load struct {
	Monthly       float64 `yaml:"monthly" json:"monthly"`
	PeakPerSecond float64 `yaml:"peakPerSecond" json:"peakPerSecond"`
	Sizes         []Sized `yaml:"-" json:"sizes,omitempty"`
}

// Sized is the part of a load whose operations are KB each.
type Sized struct {
	KB            float64 `json:"kb"`
	Monthly       float64 `json:"monthly"`
	PeakPerSecond float64 `json:"peakPerSecond"`
}

// Add returns the sum of two loads, sizes of the same KB together.
func (l Load) Add(o Load) Load {
	out := Load{Monthly: l.Monthly + o.Monthly, PeakPerSecond: l.PeakPerSecond + o.PeakPerSecond}
	for _, s := range append(append([]Sized(nil), l.Sizes...), o.Sizes...) {
		out.Sizes = addSized(out.Sizes, s)
	}
	return out
}

func addSized(sizes []Sized, s Sized) []Sized {
	for i := range sizes {
		if sizes[i].KB == s.KB {
			sizes[i].Monthly += s.Monthly
			sizes[i].PeakPerSecond += s.PeakPerSecond
			return sizes
		}
	}
	return append(sizes, s)
}

// Scale multiplies the load and each of its sizes.
func (l Load) Scale(f float64) Load {
	out := Load{Monthly: l.Monthly * f, PeakPerSecond: l.PeakPerSecond * f}
	for _, s := range l.Sizes {
		out.Sizes = append(out.Sizes, Sized{KB: s.KB, Monthly: s.Monthly * f, PeakPerSecond: s.PeakPerSecond * f})
	}
	return out
}

// Plain is the load without its sizes: what a node passes on is its own work,
// not the size of what it received.
func (l Load) Plain() Load { return Load{Monthly: l.Monthly, PeakPerSecond: l.PeakPerSecond} }

// Sized makes every operation of the load KB in size.
func (l Load) Sized(kb float64) Load {
	p := l.Plain()
	p.Sizes = []Sized{{KB: kb, Monthly: l.Monthly, PeakPerSecond: l.PeakPerSecond}}
	return p
}

// Units counts the load in billing units of stepKB: each operation of a known
// size is rounded up on its own; the rest is taken as fallbackKB (ok is false
// when some is and fallbackKB is unknown). Every operation is at least one unit.
func (l Load) Units(stepKB float64, fallbackKB *float64) (monthly, peak float64, ok bool) {
	per := func(kb float64) float64 {
		if kb <= 0 || stepKB <= 0 {
			return 1
		}
		n := kb / stepKB
		if c := float64(int64(n)); c < n {
			return c + 1
		}
		return n
	}
	restM, restP := l.Monthly, l.PeakPerSecond
	for _, s := range l.Sizes {
		monthly += s.Monthly * per(s.KB)
		peak += s.PeakPerSecond * per(s.KB)
		restM -= s.Monthly
		restP -= s.PeakPerSecond
	}
	if restM <= 1e-9*l.Monthly && restP <= 1e-9*l.PeakPerSecond {
		return monthly, peak, true
	}
	if fallbackKB == nil {
		return monthly, peak, false
	}
	return monthly + restM*per(*fallbackKB), peak + restP*per(*fallbackKB), true
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
	// KB is the size of one downstream operation, the data it moves both
	// ways. The target counts billing units by it (a 25 KB read is seven 4 KB
	// read units), and between two nodes of one group it is the traffic the
	// group reads (zone crossings).
	KB *float64 `yaml:"kb,omitempty" json:"kb,omitempty"`
	// Ops lists what one upstream unit does to the target when it is more
	// than one kind of work: a Query of 25 KB and a write of 2 KB one call in
	// ten. Kind, PerUnit and KB are the one-operation short form.
	Ops  []Op   `yaml:"ops,omitempty" json:"ops,omitempty"`
	Note string `yaml:"note,omitempty" json:"note,omitempty"`
}

// Op is one kind of work an edge does per upstream unit.
type Op struct {
	Kind    string   `yaml:"kind,omitempty" json:"kind,omitempty"`
	PerUnit *float64 `yaml:"perUnit,omitempty" json:"perUnit,omitempty"`
	KB      *float64 `yaml:"kb,omitempty" json:"kb,omitempty"`
}

// Factor returns PerUnit, defaulting to 1.
func (o Op) Factor() float64 {
	if o.PerUnit == nil {
		return 1
	}
	return *o.PerUnit
}

// Operations is the edge's work: its ops, or the one its short form says.
func (e Edge) Operations() []Op {
	if len(e.Ops) > 0 {
		return e.Ops
	}
	return []Op{{Kind: e.Kind, PerUnit: e.PerUnit, KB: e.KB}}
}

// Factor returns PerUnit, defaulting to 1.
func (e Edge) Factor() float64 {
	if e.PerUnit == nil {
		return 1
	}
	return *e.PerUnit
}
