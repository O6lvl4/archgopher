package infer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/O6lvl4/archgopher/terraform/eval"
)

// Coverage counts the managed resources of a configuration by what import
// makes of them, per type.
type Coverage struct {
	// Read resources are nodes a scouter reads.
	Read map[string]int `json:"read"`
	// Free resources cost nothing by themselves, or only connect or place
	// nodes (integrations, attachments, networks drawn as frames).
	Free map[string]int `json:"free"`
	// Unpriced resources may cost something archgopher cannot read yet:
	// nodes without a scouter, and types it knows nothing about.
	Unpriced map[string]int `json:"unpriced"`
}

// Cover classifies every managed resource. Data sources read what is managed
// elsewhere and are left out, as in Build.
func Cover(ev *eval.Evaluated, rules Rules) Coverage {
	c := Coverage{Read: map[string]int{}, Free: map[string]int{}, Unpriced: map[string]int{}}
	structural := map[string]bool{}
	for _, l := range rules.Links {
		structural[l.Type] = true
	}
	for t := range rules.Aliases {
		structural[t] = true
	}
	for _, r := range ev.Resources {
		if r.Mode == "data" {
			continue
		}
		_, scouted := rules.Scouters[r.Type]
		_, boundary := rules.Boundaries[r.Type]
		switch {
		case rules.NodeTypes[r.Type] && scouted:
			c.Read[r.Type]++
		case rules.NodeTypes[r.Type]:
			c.Unpriced[r.Type]++
		case rules.Free[r.Type] || boundary || structural[r.Type]:
			c.Free[r.Type]++
		default:
			c.Unpriced[r.Type]++
		}
	}
	return c
}

// Summary is one line for people: counts, and the unpriced types by name.
func (c Coverage) Summary() string {
	s := fmt.Sprintf("%d resources read, %d free, %d without a price yet", total(c.Read), total(c.Free), total(c.Unpriced))
	if len(c.Unpriced) == 0 {
		return s
	}
	types := make([]string, 0, len(c.Unpriced))
	for t, n := range c.Unpriced {
		if n > 1 {
			t = fmt.Sprintf("%s ×%d", t, n)
		}
		types = append(types, t)
	}
	sort.Strings(types)
	return s + ": " + strings.Join(types, ", ")
}

func total(m map[string]int) int {
	n := 0
	for _, v := range m {
		n += v
	}
	return n
}
