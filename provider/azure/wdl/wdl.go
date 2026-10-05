// Package wdl reads Logic Apps actions written in the Workflow Definition
// Language for what a Consumption workflow bills: each action that runs is
// metered, a built-in action at one rate and a managed connector call at
// another (https://azure.microsoft.com/pricing/details/logic/, checked on
// 2026-10-05).
//
// A condition or a switch takes its longest branch, so the count is the most
// a run bills; a scope runs all its actions. A loop (Foreach, Until) runs its
// actions a number of times the definition does not say, and an action type
// this package does not know is refused rather than guessed.
package wdl

import (
	"fmt"
	"sort"
)

// Actions is what one run of some actions bills.
type Actions struct {
	BuiltIn   float64
	Connector float64
}

// Plus adds the actions of b.
func (a Actions) Plus(b Actions) Actions {
	return Actions{a.BuiltIn + b.BuiltIn, a.Connector + b.Connector}
}

// heavier orders branches by connector calls first, the dearer meter.
func (a Actions) heavier(b Actions) bool {
	if a.Connector != b.Connector {
		return a.Connector > b.Connector
	}
	return a.BuiltIn > b.BuiltIn
}

// connectorTypes call a managed connector; every other known type is built in.
var connectorTypes = map[string]bool{"ApiConnection": true, "ApiConnectionWebhook": true}

// builtInTypes are the built-in action and trigger types without actions inside.
var builtInTypes = map[string]bool{
	"Http": true, "HttpWebhook": true, "Request": true, "Recurrence": true, "Response": true,
	"Compose": true, "Query": true, "Select": true, "Table": true, "Join": true, "ParseJson": true,
	"InitializeVariable": true, "SetVariable": true, "IncrementVariable": true, "DecrementVariable": true,
	"AppendToArrayVariable": true, "AppendToStringVariable": true,
	"Function": true, "Workflow": true, "Wait": true, "Terminate": true, "Expression": true,
	"JavaScriptCode": true, "XmlValidation": true, "Xslt": true, "FlatFileEncoding": true, "FlatFileDecoding": true,
	"IntegrationAccountArtifactLookup": true, "Batch": true, "SendToBatch": true,
}

// Count reads one action or trigger, with the actions inside it.
func Count(name string, doc any) (Actions, error) {
	m, ok := doc.(map[string]any)
	if !ok {
		return Actions{}, fmt.Errorf("action %q is not an object", name)
	}
	t, _ := m["type"].(string)
	switch {
	case connectorTypes[t]:
		return Actions{Connector: 1}, nil
	case builtInTypes[t]:
		return Actions{BuiltIn: 1}, nil
	case t == "Scope":
		inner, err := all(m["actions"])
		return Actions{BuiltIn: 1}.Plus(inner), err
	case t == "If":
		elseActions, _ := m["else"].(map[string]any)
		inner, err := longest(m["actions"], elseActions["actions"])
		return Actions{BuiltIn: 1}.Plus(inner), err
	case t == "Switch":
		var branches []any
		cases, _ := m["cases"].(map[string]any)
		for _, k := range sortedKeys(cases) {
			c, _ := cases[k].(map[string]any)
			branches = append(branches, c["actions"])
		}
		def, _ := m["default"].(map[string]any)
		inner, err := longest(append(branches, def["actions"])...)
		return Actions{BuiltIn: 1}.Plus(inner), err
	case t == "Foreach" || t == "Until":
		return Actions{}, fmt.Errorf("action %q is a %s loop: its actions run a number of times the definition does not say", name, t)
	}
	return Actions{}, fmt.Errorf("action %q has type %q, which is not counted yet", name, t)
}

// all counts every action of a set.
func all(actions any) (Actions, error) {
	m, _ := actions.(map[string]any)
	var sum Actions
	for _, k := range sortedKeys(m) {
		a, err := Count(k, m[k])
		if err != nil {
			return Actions{}, err
		}
		sum = sum.Plus(a)
	}
	return sum, nil
}

// longest counts the branch that bills the most.
func longest(branches ...any) (Actions, error) {
	var best Actions
	for _, b := range branches {
		a, err := all(b)
		if err != nil {
			return Actions{}, err
		}
		if a.heavier(best) {
			best = a
		}
	}
	return best, nil
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
