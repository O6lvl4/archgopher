package azure

import (
	"path/filepath"
	"testing"

	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/terraform/eval"
	"github.com/O6lvl4/archgopher/terraform/infer"
)

// Numbers that point at blocks count them, a
// Front Door calls the WAF policy its frontends link, and DNS records and
// Traffic Manager endpoints call nothing.
func TestDNSAndEdgeImport(t *testing.T) {
	ev, err := eval.Evaluate(filepath.Join("testdata", "dnsedge"), eval.Options{})
	if err != nil {
		t.Fatal(err)
	}
	spec, _ := infer.Build(ev, TerraformRules(), "dnsedge")
	attrs := map[string]map[string]any{}
	for _, n := range spec.Nodes {
		attrs[n.ID] = n.Attributes
	}
	checkAttributes(t, attrs, map[string]map[string]any{
		"fd":  {"routing_rules": 2.0, "frontend_hosts": 1.0},
		"waf": {"custom_rules": 1.0, "managed_rulesets": 2.0},
		"mx":  {"records": 2.0},
		"tm":  {"traffic_view_enabled": true},
	})
	if got, ok := attrs["www"]["records"].([]any); !ok || len(got) != 3 {
		t.Errorf("www.records: want 3 addresses, got %v", attrs["www"]["records"])
	}
	for _, e := range spec.Edges {
		if e.From == "mx" || e.From == "www" || e.From == "ep" {
			t.Errorf("%s calls %s; records and endpoints call nothing", e.From, e.To)
		}
	}
	checkEdges(t, spec.Edges, [][2]string{{"users", "fd"}, {"fd", "waf"}, {"fd", "app"}})
}

// checkAttributes compares the attributes of nodes, by id, with what they should read.
func checkAttributes(t *testing.T, attrs, want map[string]map[string]any) {
	t.Helper()
	for id, w := range want {
		for k, v := range w {
			if got := attrs[id][k]; got != v {
				t.Errorf("%s.%s: want %v, got %v", id, k, v, got)
			}
		}
	}
}

// checkEdges reports every edge, from and to, that is missing.
func checkEdges(t *testing.T, edges []model.Edge, want [][2]string) {
	t.Helper()
	have := map[[2]string]bool{}
	for _, e := range edges {
		have[[2]string{e.From, e.To}] = true
	}
	for _, e := range want {
		if !have[e] {
			t.Errorf("missing edge %s -> %s (have %v)", e[0], e[1], edges)
		}
	}
}
