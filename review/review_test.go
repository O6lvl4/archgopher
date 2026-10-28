package review

import (
	"strings"
	"testing"

	"github.com/O6lvl4/archgopher/model"
)

var oneQueue = Rule{ID: "one-queue", Pillar: "test", Title: "one queue", FanIn: &FanIn{Node: "fn", From: "queue", Max: 1}}

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
			got := Find(model.Spec{Nodes: c.nodes, Edges: c.edges}, []Rule{oneQueue})
			if len(got) != c.want {
				t.Errorf("got %d findings %v, want %d", len(got), got, c.want)
			}
		})
	}
}

func TestTextNamesTheShape(t *testing.T) {
	spec := model.Spec{Nodes: []model.Node{{ID: "q", Type: "queue", Instances: 2}, {ID: "f", Type: "fn"}}, Edges: []model.Edge{{From: "q", To: "f"}}}
	text := Text(Find(spec, []Rule{oneQueue}))
	if !strings.Contains(text, "fed by 2 queue (q ×2)") {
		t.Errorf("text = %q", text)
	}
	if Text(nil) != "No departures from the known practices.\n" {
		t.Errorf("empty text = %q", Text(nil))
	}
}
