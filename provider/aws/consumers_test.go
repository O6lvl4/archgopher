package aws

import (
	"path/filepath"
	"testing"

	"github.com/O6lvl4/archgopher/terraform/eval"
	"github.com/O6lvl4/archgopher/terraform/infer"
)

// TestEventSourceMappingWinsTheCycle keeps the queue → function edge an event
// source mapping declares when the function's role can also send to the
// queue: the cycle is broken on the inferred send edge.
func TestEventSourceMappingWinsTheCycle(t *testing.T) {
	ev, err := eval.Evaluate(filepath.Join("testdata", "consumers"), eval.Options{})
	if err != nil {
		t.Fatal(err)
	}
	spec, _ := infer.Build(ev, TerraformRules(), "test")
	has := map[[2]string]bool{}
	for _, e := range spec.Edges {
		has[[2]string{e.From, e.To}] = true
	}
	if !has[[2]string{"work", "worker"}] {
		t.Errorf("no edge work -> worker; edges %v", spec.Edges)
	}
	if has[[2]string{"worker", "work"}] {
		t.Errorf("edge worker -> work kept; it closes the cycle the mapping declares")
	}
}
