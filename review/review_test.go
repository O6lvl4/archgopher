package review

import (
	"strings"
	"testing"

	"github.com/O6lvl4/archgopher/model"
)

var oneQueue = Rule{ID: "one-queue", Pillar: "test", Title: "one queue", FanIn: &FanIn{Node: "fn", From: "queue", Max: 1}}

func graph(nodes []model.Node, edges []model.Edge) Input {
	return Input{Spec: model.Spec{Nodes: nodes, Edges: edges}}
}

func TestFanInCountsInstances(t *testing.T) {
	cases := []struct {
		name  string
		nodes []model.Node
		edges []model.Edge
		want  int
	}{
		{"one queue", []model.Node{{ID: "q", Type: "queue"}, {ID: "f", Type: "fn"}}, []model.Edge{{From: "q", To: "f"}}, 0},
		{"two queues", []model.Node{{ID: "a", Type: "queue"}, {ID: "b", Type: "queue"}, {ID: "f", Type: "fn"}}, []model.Edge{{From: "a", To: "f"}, {From: "b", To: "f"}}, 1},
		{"for_each of two", []model.Node{{ID: "q", Type: "queue", Instances: 2}, {ID: "f", Type: "fn"}}, []model.Edge{{From: "q", To: "f"}}, 1},
		{"unknown count reads as one", []model.Node{{ID: "q", Type: "queue", Instances: model.UnknownInstances}, {ID: "f", Type: "fn"}}, []model.Edge{{From: "q", To: "f"}}, 0},
		{"function sends to two", []model.Node{{ID: "a", Type: "queue"}, {ID: "b", Type: "queue"}, {ID: "f", Type: "fn"}}, []model.Edge{{From: "f", To: "a"}, {From: "f", To: "b"}}, 0},
		{"same pair twice", []model.Node{{ID: "q", Type: "queue"}, {ID: "f", Type: "fn"}}, []model.Edge{{From: "q", To: "f"}, {From: "q", To: "f"}}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Find(graph(c.nodes, c.edges), []Rule{oneQueue})
			if len(got) != c.want {
				t.Errorf("got %d findings %v, want %d", len(got), got, c.want)
			}
		})
	}
}

func seven() *float64 { n := 7.0; return &n }

func TestSetting(t *testing.T) {
	cases := []struct {
		name string
		s    Setting
		r    Resource
		want bool // departs
	}{
		{"equals met", Setting{Path: "on", Equals: true, Default: false}, Resource{Attrs: map[string]any{"on": true}}, false},
		{"equals by default", Setting{Path: "on", Equals: true, Default: false}, Resource{}, true},
		{"unknown is not judged", Setting{Path: "on", Equals: true, Default: false}, Resource{Unknown: []string{"on"}}, false},
		{"set by reference", Setting{Path: "dlq", Set: true}, Resource{Refs: map[string][]string{"dlq": {"q2"}}}, false},
		{"not set", Setting{Path: "dlq", Set: true}, Resource{}, true},
		{"at least in a block", Setting{Path: "b.days", AtLeast: seven()}, Resource{Attrs: map[string]any{"b": []any{map[string]any{"days": 3}}}}, true},
		{"one of", Setting{Path: "m", OneOf: []any{"A", "B"}}, Resource{Attrs: map[string]any{"m": "B"}}, false},
		{"contains", Setting{Path: "l", Contains: "X"}, Resource{Attrs: map[string]any{"l": []any{"X"}}}, false},
		{"when below", Setting{Path: "l", Contains: "X", WhenPath: "n", WhenAbove: 1, WhenDefault: 10}, Resource{Attrs: map[string]any{"n": 1}}, false},
		{"when by default", Setting{Path: "l", Contains: "X", WhenPath: "n", WhenAbove: 1, WhenDefault: 10}, Resource{}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.s.Type, c.r.Type, c.r.Address = "t", "t", "t.x"
			s := c.s
			got := Find(Input{Resources: []Resource{c.r}}, []Rule{{ID: "r", Setting: &s}})
			if (len(got) > 0) != c.want {
				t.Errorf("got %v, want departure %v", got, c.want)
			}
		})
	}
}

func TestScope(t *testing.T) {
	queue := func(addr string, refs map[string][]string) Resource {
		return Resource{Address: addr, Type: "queue", Refs: refs}
	}
	rule := Rule{ID: "dlq", Setting: &Setting{Type: "queue", Path: "redrive", Set: true, Scope: Scope{Except: []By{{Type: "queue", Path: "redrive"}}}}}
	in := Input{Resources: []Resource{queue("main", map[string][]string{"redrive": {"dead"}}), queue("dead", nil), queue("lonely", nil)}}
	got := Find(in, []Rule{rule})
	if len(got) != 1 || got[0].Node != "lonely" {
		t.Errorf("got %v, want only lonely", got)
	}
	rule.Setting.Scope = Scope{UnlessAny: []string{"registry"}}
	in.Resources = append(in.Resources, Resource{Address: "reg", Type: "registry"})
	if got := Find(in, []Rule{rule}); len(got) != 0 {
		t.Errorf("UnlessAny: got %v", got)
	}
}

func TestCompanionByReferenceNameOrVariant(t *testing.T) {
	rule := Rule{ID: "alarm", Companion: &Companion{Type: "queue", By: []By{{Type: "alarm", Path: "dimensions"}}, NameAttr: "name"}}
	in := Input{Resources: []Resource{
		{Address: "q1", Type: "queue", Attrs: map[string]any{"name": "one"}},
		{Address: "q2", Type: "queue", Attrs: map[string]any{"name": "two"}},
		{Address: "q3", Type: "queue", Attrs: map[string]any{"name": "three"}},
		{Address: "q4", Type: "queue", Attrs: map[string]any{"name": "four"}},
		{Address: "a1", Type: "alarm", Refs: map[string][]string{"dimensions": {"q1"}}},
		{Address: "a2", Type: "alarm", Attrs: map[string]any{"dimensions": map[string]any{"QueueName": "two"}},
			Variants: []map[string]any{{"dimensions": map[string]any{"QueueName": "three"}}}},
	}}
	got := Find(in, []Rule{rule})
	if len(got) != 1 || got[0].Node != "q4" {
		t.Errorf("got %v, want only q4", got)
	}
}

func TestRatio(t *testing.T) {
	rule := Rule{ID: "vis", Ratio: &Ratio{Type: "link", Min: 6,
		Left:  End{Ref: "source", Attr: "visibility", Default: 30},
		Right: End{Ref: "fn", Attr: "timeout", Default: 3}}}
	res := func(visibility any) Input {
		q := Resource{Address: "q", Type: "queue", Attrs: map[string]any{}}
		if visibility != nil {
			q.Attrs["visibility"] = visibility
		}
		return Input{Resources: []Resource{q,
			{Address: "f", Type: "fn", Attrs: map[string]any{"timeout": 60}},
			{Address: "l", Type: "link", Refs: map[string][]string{"source": {"q"}, "fn": {"f"}}}}}
	}
	if got := Find(res(360), []Rule{rule}); len(got) != 0 {
		t.Errorf("360 vs 60: got %v", got)
	}
	if got := Find(res(300), []Rule{rule}); len(got) != 1 {
		t.Errorf("300 vs 60: got %v", got)
	}
	if got := Find(res(nil), []Rule{rule}); len(got) != 1 {
		t.Errorf("default 30 vs 60: got %v", got)
	}
}

func TestSharedCountsInstances(t *testing.T) {
	rule := Rule{ID: "role", Shared: &Shared{Type: "role", By: By{Type: "fn", Path: "role"}}}
	in := Input{Resources: []Resource{
		{Address: "shared", Type: "role"},
		{Address: "each", Type: "role", Instances: 2},
		{Address: "f1", Type: "fn", Refs: map[string][]string{"role": {"shared"}}},
		{Address: "f2", Type: "fn", Refs: map[string][]string{"role": {"shared"}}},
		{Address: "f3", Type: "fn", Refs: map[string][]string{"role": {"each"}}},
		{Address: "f4", Type: "fn", Refs: map[string][]string{"role": {"each"}}},
	}}
	got := Find(in, []Rule{rule})
	if len(got) != 1 || got[0].Node != "shared" {
		t.Errorf("got %v, want only shared", got)
	}
}

func TestTextGroupsByRule(t *testing.T) {
	in := graph([]model.Node{{ID: "q", Type: "queue", Instances: 2}, {ID: "f", Type: "fn"}}, []model.Edge{{From: "q", To: "f"}})
	text := Text(Find(in, []Rule{oneQueue}))
	if !strings.Contains(text, "## one-queue [test] ×1") || !strings.Contains(text, "fed by 2 queue (q ×2)") {
		t.Errorf("text = %q", text)
	}
	if Text(nil) != "No departures from the known practices.\n" {
		t.Errorf("empty text = %q", Text(nil))
	}
}
