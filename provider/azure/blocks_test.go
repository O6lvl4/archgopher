package azure

import (
	"path/filepath"
	"testing"

	"github.com/O6lvl4/archgopher/terraform/eval"
	"github.com/O6lvl4/archgopher/terraform/infer"
)

// A number attribute that points at a block reads how many are written,
// dynamic blocks included; none written leaves the default.
func TestNumberAttributesCountBlocks(t *testing.T) {
	ev, err := eval.Evaluate(filepath.Join("testdata", "georeplications"), eval.Options{})
	if err != nil {
		t.Fatal(err)
	}
	spec, _ := infer.Build(ev, TerraformRules(), "georeplications")
	got := map[string]any{}
	for _, n := range spec.Nodes {
		got[n.ID] = n.Attributes["replicas"]
	}
	if got["acr"] != 2.0 {
		t.Errorf("acr replicas: want 2, got %v", got["acr"])
	}
	if got["plain"] != nil {
		t.Errorf("plain replicas: want unset, got %v", got["plain"])
	}
}
