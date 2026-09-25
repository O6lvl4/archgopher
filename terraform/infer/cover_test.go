package infer

import (
	"testing"

	"github.com/O6lvl4/archgopher/scouter"
	"github.com/O6lvl4/archgopher/terraform/eval"
)

// Coverage splits managed resources into read, free and unpriced. Links,
// aliases and boundaries only connect or place nodes, so they are free; a
// node type without a scouter and an unknown type are unpriced; data sources
// are left out.
func TestCover(t *testing.T) {
	rules := Rules{
		NodeTypes:  map[string]bool{"fn": true, "lb": true},
		Scouters:   scouter.Registry{"fn": nil},
		Free:       map[string]bool{"role": true},
		Links:      []Link{{Type: "integration"}},
		Aliases:    map[string]string{"listener": "load_balancer_arn"},
		Boundaries: map[string]string{"vpc": "VPC"},
	}
	ev := &eval.Evaluated{}
	for _, r := range []struct{ typ, mode string }{
		{"fn", "managed"}, {"fn", "managed"}, {"lb", "managed"}, {"role", "managed"}, {"integration", "managed"},
		{"listener", "managed"}, {"vpc", "managed"}, {"mystery", "managed"}, {"fn", "data"},
	} {
		ev.Resources = append(ev.Resources, &eval.Resource{Type: r.typ, Mode: r.mode})
	}
	c := Cover(ev, rules)
	if c.Read["fn"] != 2 || len(c.Read) != 1 {
		t.Errorf("read %v", c.Read)
	}
	if total(c.Free) != 4 {
		t.Errorf("free %v", c.Free)
	}
	if c.Unpriced["lb"] != 1 || c.Unpriced["mystery"] != 1 || len(c.Unpriced) != 2 {
		t.Errorf("unpriced %v", c.Unpriced)
	}
	if got, want := c.Summary(), "2 resources read, 4 free, 2 without a price yet: lb, mystery"; got != want {
		t.Errorf("summary %q, want %q", got, want)
	}
}
