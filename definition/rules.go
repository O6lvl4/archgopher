package definition

import (
	"github.com/O6lvl4/archgopher/scouter"
	"github.com/O6lvl4/archgopher/terraform/infer"
)

// Rules is the resource's share of the Terraform rules. Schedules name the
// attribute only; the provider supplies the parser for its schedule syntax.
func (r *Resource) Rules() infer.Rules {
	f, t := r.File, r.File.Terraform
	rules := infer.Rules{
		NodeTypes:        map[string]bool{},
		FrontDoors:       map[string]bool{},
		Mentioned:        map[string]bool{},
		Passive:          map[string]bool{},
		Aliases:          t.Aliases,
		Forward:          map[string]string{},
		FrontDoorAliases: t.FrontDoorAliases,
		Schedules:        map[string]string{},
		IgnoreRefs:       t.IgnoreRefs,
		Scouters:         scouter.Registry{f.Type: r},
	}
	switch {
	case f.Boundary:
		rules.Boundaries = map[string]string{f.Type: f.Label}
	case !f.External:
		rules.NodeTypes[f.Type] = true
	}
	if t.FrontDoor {
		rules.FrontDoors[f.Type] = true
	}
	if t.Mentioned {
		rules.Mentioned[f.Type] = true
	}
	if t.Passive {
		rules.Passive[f.Type] = true
	}
	if t.Forward != "" {
		rules.Forward[f.Type] = t.Forward
	}
	if t.Schedule != "" {
		rules.Schedules[f.Type] = t.Schedule
	}
	for _, l := range t.Links {
		rules.Links = append(rules.Links, infer.Link{Type: l.Type, From: l.From, To: l.To})
	}
	return rules
}
