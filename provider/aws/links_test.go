package aws

import (
	"path/filepath"
	"testing"

	"github.com/O6lvl4/archgopher/terraform/eval"
	"github.com/O6lvl4/archgopher/terraform/infer"
)

// TestLinksThroughModulesAndCarriers draws the calls community-style code
// hides: an event source a module makes with for_each from a map of objects,
// and a database reached through the secret or parameter holding its endpoint.
func TestLinksThroughModulesAndCarriers(t *testing.T) {
	ev, err := eval.Evaluate(filepath.Join("testdata", "links"), eval.Options{})
	if err != nil {
		t.Fatal(err)
	}
	spec, _ := infer.Build(ev, TerraformRules(), "test")
	has := map[[2]string]bool{}
	for _, e := range spec.Edges {
		has[[2]string{e.From, e.To}] = true
	}
	for _, want := range [][2]string{{"jobs", "worker"}, {"via_secret", "db"}, {"via_parameter", "db"}, {"via_parameter", "db_host"}} {
		if !has[want] {
			t.Errorf("no edge %s -> %s; edges %v", want[0], want[1], spec.Edges)
		}
	}
	if !has[[2]string{"events", "worker"}] {
		t.Errorf("no edge events -> worker; edges %v", spec.Edges)
	}
	if has[[2]string{"failures", "worker"}] {
		t.Error("a failure destination is not an event source")
	}
	if has[[2]string{"db_host", "db"}] {
		t.Error("a parameter holding an endpoint does not call the database")
	}
}
