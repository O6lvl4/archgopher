package infer

import (
	"fmt"
	"strings"

	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/scouter"
	"github.com/O6lvl4/archgopher/terraform/eval"
	"github.com/O6lvl4/archgopher/traffic"
)

// node turns a resource into a node: the fields its scouter reads, the
// assumptions it needs, a schedule as traffic, and notes on what the
// declaration cannot say.
func (b *builder) node(r *eval.Resource) model.Node {
	n := model.Node{ID: b.id(r), Type: r.Type, Address: r.Address, Attributes: map[string]any{}}
	if s, ok := b.rules.Scouters[r.Type]; ok {
		b.readFields(&n, r, s)
	} else {
		n.Note = "No scouter reads " + r.Type + " yet; load passes through."
	}
	if attr, ok := b.rules.Schedules[r.Type]; ok {
		b.readSchedule(&n, r, attr)
	}
	if note := instancesNote(r.Instances); note != "" {
		n.Note = join(n.Note, note)
	}
	if len(n.Attributes) == 0 {
		n.Attributes = nil
	}
	return n
}

// readFields copies the attributes the scouter reads and asks for its
// required assumptions, which Terraform cannot answer.
func (b *builder) readFields(n *model.Node, r *eval.Resource, s scouter.Scouter) {
	for _, f := range s.Attributes() {
		if v := b.readAttribute(r, f); v != nil {
			n.Attributes[f.Key] = v
		}
	}
	for _, f := range s.Assumptions() {
		if !f.Required {
			continue
		}
		if n.Assumptions == nil {
			n.Assumptions = map[string]any{}
		}
		n.Assumptions[f.Key] = nil
	}
}

// readSchedule takes the schedule expression at attr as the node's traffic.
// The schedule itself is the traffic, so the declaration keeps what
// Terraform says and follows it when it changes.
func (b *builder) readSchedule(n *model.Node, r *eval.Resource, attr string) {
	expr, _ := r.Attrs[attr].(string)
	if _, err := traffic.Parse(expr); err != nil {
		b.warnings = append(b.warnings, fmt.Sprintf("%s: %v", r.Address, err))
		return
	}
	n.Traffic = &model.Traffic{Schedule: expr}
}

// instancesNote says how one node stands for a counted resource, or "" for
// a single instance.
func instancesNote(instances int) string {
	switch {
	case instances > 1:
		return fmt.Sprintf("%d instances (count or for_each); one node carries their combined load.", instances)
	case instances < 0:
		return "Instance count is unknown before apply; one node carries the combined load."
	}
	return ""
}

// id derives a short, stable id: the resource name, the module name for
// generic names like "this", and the type as a suffix on collision.
func (b *builder) id(r *eval.Resource) string {
	if b.used == nil {
		b.used = map[string]bool{UsersID: true}
	}
	base := idBase(r)
	id := base
	if b.used[id] {
		id = base + "-" + shortType(r.Type)
	}
	for i := 2; b.used[id]; i++ {
		id = fmt.Sprintf("%s-%d", base, i)
	}
	b.used[id] = true
	b.ids[r.Address] = id
	return id
}

// idBase is the resource name, prefixed with its module's name, or the
// module's name alone for generic resource names.
func idBase(r *eval.Resource) string {
	if r.Module == "" {
		return r.Name
	}
	mod := r.Module[strings.LastIndex(r.Module, ".")+1:]
	switch r.Name {
	case "this", "main", "default", "self":
		return mod
	}
	return mod + "." + r.Name
}

// shortType is the service part of a type: aws_sqs_queue -> sqs.
func shortType(t string) string {
	parts := strings.Split(t, "_")
	if len(parts) > 1 {
		return parts[1]
	}
	return t
}

func join(a, b string) string {
	if a == "" {
		return b
	}
	return a + " " + b
}
