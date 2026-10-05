package azure

import (
	"errors"
	"fmt"
	"slices"

	"github.com/O6lvl4/archgopher/provider/azure/wdl"
	"github.com/O6lvl4/archgopher/terraform/eval"
	"github.com/O6lvl4/archgopher/terraform/infer"
)

// readers derive what no attribute path can read.
var readers = map[string]infer.Reader{"azurerm_logic_app_workflow": logicAppActions}

// fixedActions are the action and trigger types that run one built-in action;
// custom ones say what they run in their body.
var fixedActions = map[string]bool{
	"azurerm_logic_app_action_http":          true,
	"azurerm_logic_app_trigger_http_request": true,
	"azurerm_logic_app_trigger_recurrence":   true,
}

var customActions = map[string]bool{
	"azurerm_logic_app_action_custom":  true,
	"azurerm_logic_app_trigger_custom": true,
}

// logicAppActions counts the actions one run of a Consumption workflow bills:
// its trigger and every action that names it with logic_app_id.
func logicAppActions(g *infer.Graph, wf *eval.Resource) (map[string]any, error) {
	var sum wdl.Actions
	found := false
	for _, r := range g.Resources() {
		if !(fixedActions[r.Type] || customActions[r.Type]) || !slices.Contains(r.Refs["logic_app_id"], wf.Address) {
			continue
		}
		found = true
		a, err := actionOf(r)
		if err != nil {
			return nil, fmt.Errorf("%s: %w; set builtInActionsPerRun and standardCallsPerRun", r.Address, err)
		}
		switch {
		case r.Instances == eval.UnknownInstances:
			return nil, fmt.Errorf("%s: its count is not known before apply; set builtInActionsPerRun and standardCallsPerRun", r.Address)
		case r.Instances > 1:
			a = wdl.Actions{BuiltIn: a.BuiltIn * float64(r.Instances), Connector: a.Connector * float64(r.Instances)}
		}
		sum = sum.Plus(a)
	}
	if !found {
		return nil, errors.New("no action or trigger in Terraform names this workflow; set builtInActionsPerRun")
	}
	return map[string]any{"builtInActions": sum.BuiltIn, "connectorActions": sum.Connector}, nil
}

func actionOf(r *eval.Resource) (wdl.Actions, error) {
	if fixedActions[r.Type] {
		return wdl.Actions{BuiltIn: 1}, nil
	}
	if r.Body == nil || r.Scope == nil {
		return wdl.Actions{}, errors.New("its body cannot be read")
	}
	a, ok := r.Body.Attributes["body"]
	if !ok {
		return wdl.Actions{}, errors.New("it has no body")
	}
	doc, ok := r.Scope.Document(a.Expr)
	if !ok {
		return wdl.Actions{}, errors.New("its body cannot be read before apply")
	}
	return wdl.Count(r.Name, doc)
}
