// Package definition reads a cloud resource written as data: one directory
// holds resource.yaml (what the resource accepts, how its load turns into
// facet readings, its Terraform rules and IAM actions), its books and its
// test cases. A definition compiles into a scouter, so a new resource needs
// no Go code: the facets (L2) are Go, the resources that compose them are data.
package definition

import (
	"fmt"
	"reflect"

	"github.com/O6lvl4/archgopher/facet"
	"github.com/O6lvl4/archgopher/field"
	"github.com/O6lvl4/archgopher/meter"
	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/scouter"
)

// File is resource.yaml.
type File struct {
	Type        string       `yaml:"type"`
	Label       string       `yaml:"label"`
	Category    string       `yaml:"category"`
	Description string       `yaml:"description"`
	Kinds       []string     `yaml:"kinds"`
	SLA         string       `yaml:"sla"`
	External    bool         `yaml:"external"`
	Attributes  []field.Spec `yaml:"attributes"`
	Assumptions []field.Spec `yaml:"assumptions"`
	// Includes pulls in a facet's assumption fields (logs, tokens).
	Includes []string `yaml:"includes"`
	// Let computes named values in order; later ones and readings can use them.
	Let       []map[string]string `yaml:"let"`
	Readings  []map[string]Params `yaml:"readings"`
	Terraform Terraform           `yaml:"terraform"`
	// IAM maps a kind of work to the actions that do it on this resource.
	IAM map[string][]string `yaml:"iam"`
}

// Terraform is the resource's share of the rules that turn Terraform into a graph.
type Terraform struct {
	FrontDoor bool `yaml:"frontDoor"`
	Mentioned bool `yaml:"mentioned"`
	// Passive: references from this resource are never calls (an alarm names what it watches).
	Passive          bool              `yaml:"passive"`
	Links            []Link            `yaml:"links"`
	Aliases          map[string]string `yaml:"aliases"`
	FrontDoorAliases map[string]string `yaml:"frontDoorAliases"`
	// Schedule names the attribute holding a schedule expression.
	Schedule   string   `yaml:"schedule"`
	IgnoreRefs []string `yaml:"ignoreRefs"`
}

// Link is a helper resource that connects two nodes.
type Link struct {
	Type string   `yaml:"type"`
	From string   `yaml:"from"`
	To   []string `yaml:"to"`
}

// Resource is a compiled definition. It implements scouter.Scouter.
type Resource struct {
	File        File
	attributes  []field.Field
	assumptions []field.Field
	lets        []let
	readings    []reading
}

var _ scouter.Scouter = (*Resource)(nil)

// Compile validates a definition and compiles its expressions.
func Compile(f File) (*Resource, error) {
	if f.Type == "" || len(f.Kinds) == 0 {
		return nil, fmt.Errorf("a resource needs a type and at least one kind")
	}
	r := &Resource{File: f}
	var err error
	if r.attributes, err = build(f.Attributes); err != nil {
		return nil, fmt.Errorf("%s attributes: %w", f.Type, err)
	}
	if r.assumptions, err = build(f.Assumptions); err != nil {
		return nil, fmt.Errorf("%s assumptions: %w", f.Type, err)
	}
	for _, inc := range f.Includes {
		t, ok := facet.Includes[inc]
		if !ok {
			return nil, fmt.Errorf("%s: unknown include %q", f.Type, inc)
		}
		r.assumptions = append(r.assumptions, field.FieldsOf(t)...)
	}
	for _, fd := range append(append([]field.Field(nil), r.attributes...), r.assumptions...) {
		if reserved[fd.Key] {
			return nil, fmt.Errorf("%s: %q is a name every expression already has", f.Type, fd.Key)
		}
	}
	decl := declare(r.attributes, r.assumptions)
	if r.lets, err = compileLets(f.Let, decl); err != nil {
		return nil, fmt.Errorf("%s: %w", f.Type, err)
	}
	for i, raw := range f.Readings {
		rd, err := compileReading(raw, decl)
		if err != nil {
			return nil, fmt.Errorf("%s reading %d: %w", f.Type, i+1, err)
		}
		r.readings = append(r.readings, rd)
	}
	return r, nil
}

func build(specs []field.Spec) ([]field.Field, error) {
	out := make([]field.Field, 0, len(specs))
	seen := map[string]bool{}
	for _, s := range specs {
		f, err := s.Build()
		if err != nil {
			return nil, err
		}
		if seen[f.Key] {
			return nil, fmt.Errorf("key %q is declared twice", f.Key)
		}
		seen[f.Key] = true
		out = append(out, f)
	}
	return out, nil
}

// Meta is the catalog entry.
func (r *Resource) Meta() scouter.Meta {
	f := r.File
	return scouter.Meta{Type: f.Type, Label: f.Label, Category: f.Category, Description: f.Description, Kinds: f.Kinds, SLA: f.SLA, External: f.External}
}

// Attributes lists the Terraform attributes the resource reads.
func (r *Resource) Attributes() []field.Field { return r.attributes }

// Assumptions lists the assumptions, then the latency fields every node has.
func (r *Resource) Assumptions() []field.Field {
	return append(append([]field.Field(nil), r.assumptions...), scouter.LatencyFields...)
}

// Scout decodes the node's values and runs the readings in order.
func (r *Resource) Scout(node model.Node, demand model.Demand, rec *meter.Recorder) error {
	attrs, errA := field.DecodeValues(r.attributes, node.Attributes, "attribute")
	assume, errP := field.DecodeValues(r.assumptions, node.Assumptions, "assumption", scouter.LatencyP50, scouter.LatencyP99)
	if errA != nil || errP != nil {
		return joinErrs(errA, errP)
	}
	env := runtimeEnv(rec.Region, r.attributes, r.assumptions, attrs, assume, r.File.Kinds, demand)
	for _, l := range r.lets {
		v, err := l.eval(env)
		if err != nil {
			return err
		}
		env[l.name] = v
	}
	for _, rd := range r.readings {
		stop, err := rd.run(env, rec)
		if err != nil {
			return err
		}
		if stop {
			break
		}
	}
	return rec.Err()
}

func joinErrs(errs ...error) error {
	var msg string
	for _, e := range errs {
		if e == nil {
			continue
		}
		if msg != "" {
			msg += "; "
		}
		msg += e.Error()
	}
	return fmt.Errorf("%s", msg)
}

// typeOf is the Go type an expression sees for a field: a pointer when the
// field is optional, so `x != nil` works and unset values are nil.
func typeOf(f field.Field) reflect.Type {
	var t reflect.Type
	switch f.Type {
	case field.Number:
		t = reflect.TypeFor[float64]()
	case field.Flag:
		t = reflect.TypeFor[bool]()
	case field.List:
		return reflect.TypeFor[[]string]()
	case field.Choice:
		if f.Multi {
			return reflect.TypeFor[[]string]()
		}
		t = reflect.TypeFor[string]()
	default:
		t = reflect.TypeFor[string]()
	}
	if f.Required || f.Default != nil {
		return t
	}
	return reflect.PointerTo(t)
}
