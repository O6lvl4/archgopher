package aws

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/O6lvl4/archgopher/terraform/eval"
	"github.com/O6lvl4/archgopher/terraform/infer"
)

// TestStateMachineTransitions reads the definition however it is written:
// jsonencode, a heredoc, a file through a local, a templatefile in a module.
func TestStateMachineTransitions(t *testing.T) {
	ev, err := eval.Evaluate(filepath.Join("testdata", "statemachines"), eval.Options{})
	if err != nil {
		t.Fatal(err)
	}
	spec, warnings := infer.Build(ev, TerraformRules(), "test")
	want := map[string]map[string]any{
		"inline":  {"transitions": 4.0},
		"heredoc": {"transitions": 5.0},
		// Validate, Each and Done with start and end; Process and Record per item.
		"flow":    {"transitions": 5.0, "transitionsPerItem": 2.0},
		"polling": nil,
		"express": {"type": "EXPRESS"},
	}
	for _, n := range spec.Nodes {
		w, ok := want[n.ID]
		if !ok {
			continue
		}
		delete(want, n.ID)
		if got := map[string]any(n.Attributes); !reflect.DeepEqual(got, w) && !(len(got) == 0 && len(w) == 0) {
			t.Errorf("%s attributes = %v, want %v", n.ID, got, w)
		}
	}
	if len(want) > 0 {
		t.Errorf("no nodes for %v", want)
	}
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "aws_sfn_state_machine.polling: the states loop (Wait → Check → Ready? → Wait)") {
		t.Errorf("the polling loop is not reported: %q", joined)
	}
	if strings.Contains(joined, "express") {
		t.Errorf("an express workflow needs no count: %q", joined)
	}
}
