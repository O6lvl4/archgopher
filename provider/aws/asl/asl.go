// Package asl reads a state machine written in the Amazon States Language
// for what a standard workflow bills: the state transitions of one execution.
//
// A transition is counted each time a state runs, plus the start and the end
// of the execution (the pricing page's worked examples count both). Inside a
// Parallel branch or an inline Map iteration every state counts, and starting
// an iteration of an inline Map does not. A Choice takes its longest branch,
// so the count is the most an execution bills; retries and Catch paths are
// failures and are left out.
//
// The rules follow the worked examples of
// https://aws.amazon.com/step-functions/pricing/ as checked on 2026-10-05. A
// state type this package does not know is refused rather than counted, so a
// new kind of state cannot be billed by a guess.
package asl

import (
	"errors"
	"fmt"
	"strings"
)

// Transitions is what one execution bills.
type Transitions struct {
	// PerExecution counts the states on the longest path, start and end included.
	PerExecution float64
	// PerItem counts the states every item of an inline Map runs, over the
	// Map states on that path.
	PerItem float64
}

// Count reads a definition. It refuses what the definition alone cannot
// count: states that loop, a Map inside another, a distributed Map.
func Count(doc any) (Transitions, error) {
	w, err := machine(doc, false)
	if err != nil {
		return Transitions{}, err
	}
	return Transitions{PerExecution: w.fixed + 2, PerItem: w.perItem}, nil
}

// weight is the transitions of a path: those it runs once, and those it runs
// once per Map item.
type weight struct{ fixed, perItem float64 }

func (w weight) plus(o weight) weight { return weight{w.fixed + o.fixed, w.perItem + o.perItem} }

// heavier orders paths by their per-item transitions first: with any number
// of items worth mapping, those dominate.
func (w weight) heavier(o weight) bool {
	if w.perItem != o.perItem {
		return w.perItem > o.perItem
	}
	return w.fixed > o.fixed
}

// walk finds the longest path of one state machine: the whole definition, a
// Parallel branch or a Map's item processor.
type walk struct {
	states  map[string]any
	inMap   bool
	longest map[string]weight
	onPath  []string
}

func machine(doc any, inMap bool) (weight, error) {
	m, _ := doc.(map[string]any)
	states, _ := m["States"].(map[string]any)
	start, _ := m["StartAt"].(string)
	if start == "" || states == nil {
		return weight{}, errors.New("the definition has no StartAt or States")
	}
	w := &walk{states: states, inMap: inMap, longest: map[string]weight{}}
	return w.from(start)
}

// from is the longest path from the state name to the end.
func (w *walk) from(name string) (weight, error) {
	if l, ok := w.longest[name]; ok {
		return l, nil
	}
	for i, n := range w.onPath {
		if n == name {
			loop := append(append([]string(nil), w.onPath[i:]...), name)
			return weight{}, fmt.Errorf("the states loop (%s), so how often they run is not in the definition", strings.Join(loop, " → "))
		}
	}
	st, ok := w.states[name].(map[string]any)
	if !ok {
		return weight{}, fmt.Errorf("state %q is not defined", name)
	}
	w.onPath = append(w.onPath, name)
	defer func() { w.onPath = w.onPath[:len(w.onPath)-1] }()
	self, err := w.state(name, st)
	if err != nil {
		return weight{}, err
	}
	var rest weight
	for _, next := range successors(st) {
		l, err := w.from(next)
		if err != nil {
			return weight{}, err
		}
		if l.heavier(rest) {
			rest = l
		}
	}
	w.longest[name] = self.plus(rest)
	return w.longest[name], nil
}

// state is the transitions of one state: itself, every state of its
// Parallel branches, and the states each item of its Map runs.
func (w *walk) state(name string, st map[string]any) (weight, error) {
	self := weight{fixed: 1}
	switch st["Type"] {
	case "Task", "Pass", "Wait", "Choice", "Succeed", "Fail":
	case "Parallel":
		branches, _ := st["Branches"].([]any)
		for _, b := range branches {
			bw, err := machine(b, w.inMap)
			if err != nil {
				return weight{}, fmt.Errorf("the Parallel state %q: %w", name, err)
			}
			self = self.plus(bw)
		}
	case "Map":
		proc, ok := st["ItemProcessor"].(map[string]any)
		if !ok {
			proc, _ = st["Iterator"].(map[string]any)
		}
		if cfg, _ := proc["ProcessorConfig"].(map[string]any); cfg["Mode"] == "DISTRIBUTED" {
			return weight{}, fmt.Errorf("the Map state %q is distributed: its items run as child executions, billed by their own type", name)
		}
		if w.inMap {
			return weight{}, fmt.Errorf("the Map state %q runs inside another Map, so its items per execution are not one number", name)
		}
		iw, err := machine(proc, true)
		if err != nil {
			return weight{}, fmt.Errorf("the Map state %q: %w", name, err)
		}
		self.perItem = iw.fixed
	default:
		return weight{}, fmt.Errorf("state %q has type %v, which is not counted yet", name, st["Type"])
	}
	return self, nil
}

// successors are the states that may run next on success.
func successors(st map[string]any) []string {
	var out []string
	add := func(v any) {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	switch st["Type"] {
	case "Succeed", "Fail":
		return nil
	case "Choice":
		choices, _ := st["Choices"].([]any)
		for _, c := range choices {
			if cm, ok := c.(map[string]any); ok {
				add(cm["Next"])
			}
		}
		add(st["Default"])
		return out
	}
	if end, _ := st["End"].(bool); !end {
		add(st["Next"])
	}
	return out
}
