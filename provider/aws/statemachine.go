package aws

import (
	"errors"
	"fmt"

	"github.com/O6lvl4/archgopher/provider/aws/asl"
	"github.com/O6lvl4/archgopher/terraform/eval"
	"github.com/O6lvl4/archgopher/terraform/infer"
)

// readers derive what no attribute path can read.
var readers = map[string]infer.Reader{"aws_sfn_state_machine": stateMachine}

// stateMachine reads the state transitions a standard workflow's definition
// bills per execution and per inline Map item. Express workflows bill
// requests and duration, which the definition does not tell.
func stateMachine(_ *infer.Graph, r *eval.Resource) (map[string]any, error) {
	if r.Attrs["type"] == "EXPRESS" {
		return nil, nil
	}
	doc, ok := stateMachineDefinition(r)
	if !ok {
		return nil, errors.New("the definition cannot be read before apply; set transitionsPerExecution")
	}
	t, err := asl.Count(doc)
	if err != nil {
		return nil, fmt.Errorf("%w; set transitionsPerExecution", err)
	}
	attrs := map[string]any{"transitions": t.PerExecution}
	if t.PerItem > 0 {
		attrs["transitionsPerItem"] = t.PerItem
	}
	return attrs, nil
}

// stateMachineDefinition reads the definition's structure, which is written before
// apply even when the ARNs inside it are not.
func stateMachineDefinition(r *eval.Resource) (any, bool) {
	if r.Body != nil && r.Scope != nil {
		if a, ok := r.Body.Attributes["definition"]; ok {
			return r.Scope.Document(a.Expr)
		}
	}
	return nil, false
}
