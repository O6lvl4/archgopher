package conoha

import (
	"path/filepath"
	"testing"

	"github.com/O6lvl4/archgopher/terraform/eval"
	"github.com/O6lvl4/archgopher/terraform/infer"
)

// The Aid-On fork's resources: the load balancer is a front door that calls
// the server its member names through the pool and listener, attachments
// connect the server to its additional volume and addresses, the auto backup
// names its server, names looked up through data sources (a QoS policy, a
// flavor) come along, and listeners, pools, ports, networks, DNS and
// containers are free and never nodes.
func TestForkImport(t *testing.T) {
	ev, err := eval.Evaluate(filepath.Join("testdata", "fork"), eval.Options{})
	if err != nil {
		t.Fatal(err)
	}
	spec, warnings := infer.Build(ev, TerraformRules(), "fork")
	nodes := map[string]string{}
	attrs := map[string]map[string]any{}
	for _, n := range spec.Nodes {
		nodes[n.ID] = n.Type
		attrs[n.ID] = n.Attributes
	}
	for id, typ := range map[string]string{
		"users": "entry", "web": "conohavps_instance", "boot": "conohavps_volume", "data": "conohavps_volume",
		"extra": "conohavps_additional_ip", "backup": "conohavps_instance_autobackup", "lb": "conohavps_lb_loadbalancer",
		"images": "conohavps_image_quota", "objects": "conohavps_objectstorage_quota", "api": "conohavps_instance", "api-volume": "conohavps_volume",
	} {
		if nodes[id] != typ {
			t.Errorf("node %s: want %s, got %q", id, typ, nodes[id])
		}
	}
	for _, free := range []string{"deploy", "http", "local", "assets", "example", "www"} {
		if typ, ok := nodes[free]; ok {
			t.Errorf("%s (%s) is free and should not be a node", free, typ)
		}
	}
	if len(nodes) != 11 {
		t.Errorf("want 11 nodes, got %d: %v", len(nodes), nodes)
	}
	// Names looked up through data sources come along: the QoS policy of the
	// additional addresses and the flavor of the server.
	if got := attrs["extra"]["qos_policy"]; got != "global-i_300000-o_300000" {
		t.Errorf("extra.qos_policy = %v", got)
	}
	if got := attrs["api"]["flavor_name"]; got != "g2l-t-c4m4" {
		t.Errorf("api.flavor_name = %v", got)
	}
	have := map[[2]string]bool{}
	for _, e := range spec.Edges {
		have[[2]string{e.From, e.To}] = true
	}
	for _, want := range [][2]string{{"users", "lb"}, {"lb", "web"}, {"web", "data"}, {"web", "extra"}, {"backup", "web"}} {
		if !have[want] {
			t.Errorf("missing edge %s → %s in %v", want[0], want[1], spec.Edges)
		}
	}
	for _, not := range [][2]string{{"web", "boot"}, {"web", "images"}, {"users", "web"}} {
		if have[not] {
			t.Errorf("unexpected edge %s → %s", not[0], not[1])
		}
	}
	for _, w := range warnings {
		t.Logf("warning: %s", w)
	}
}
