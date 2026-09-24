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
	// Load makes the node an entry point of the graph.
	Load     *Load     `yaml:"load,omitempty" json:"load,omitempty"`
	Note     string    `yaml:"note,omitempty" json:"note,omitempty"`
	Position *Position `yaml:"position,omitempty" json:"position,omitempty"`
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
	Note    string   `yaml:"note,omitempty" json:"note,omitempty"`
}

// Factor returns PerUnit, defaulting to 1.
func (e Edge) Factor() float64 {
	if e.PerUnit == nil {
		return 1
	}
	return *e.PerUnit
}
