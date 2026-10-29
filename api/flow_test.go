package api

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/O6lvl4/archgopher/cloud"
	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/terraform/eval"
	"github.com/O6lvl4/archgopher/terraform/infer"
)

// TestFlowReadsBacklogAndConnections: five consumers of a minute each drain
// 1/12 message a second; a burst of one a second for an hour leaves about
// 3,300 messages and eleven hours of wait, and the database sees at most five
// connections.
func TestFlowReadsBacklogAndConnections(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "flow.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	spec, err := model.ParseSpec(data)
	if err != nil {
		t.Fatal(err)
	}
	rs, err := Flow(spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 || rs[1].Scenario != "burst" {
		t.Fatalf("results %+v", rs)
	}
	steady, burst := rs[0].Queues[0], rs[1].Queues[0]
	if math.Abs(steady.Capacity-5.0/60) > 1e-9 || steady.Backlog != 0 {
		t.Errorf("steady %+v", steady)
	}
	if math.Abs(burst.Backlog-(1-5.0/60)*3600) > 1e-6 || math.Abs(burst.WaitMinutes-660) > 1e-6 {
		t.Errorf("burst %+v", burst)
	}
	if st := rs[0].Stores; len(st) != 1 || st[0].Connections != 5 || len(st[0].Uncapped) != 0 {
		t.Errorf("stores %+v", st)
	}
}

// TestEventSourceCapIsRead: a function's SQS event source with a maximum
// concurrency becomes the function's cap; one without leaves none.
func TestEventSourceCapIsRead(t *testing.T) {
	ev, err := eval.Evaluate(filepath.Join("testdata", "review"), eval.Options{})
	if err != nil {
		t.Fatal(err)
	}
	spec, _ := infer.Build(ev, cloud.TerraformRules(), "review")
	seen := 0
	for _, n := range spec.Nodes {
		if n.Type != "aws_lambda_function" {
			continue
		}
		switch n.Address {
		case "aws_lambda_function.good":
			seen++
			if n.Attributes["event_source_max_concurrency"] != 2 {
				t.Errorf("good: %v", n.Attributes)
			}
		case "aws_lambda_function.bad":
			seen++
			if n.Attributes["event_source_max_concurrency"] != nil {
				t.Errorf("bad: %v", n.Attributes)
			}
		}
	}
	if seen != 2 {
		t.Errorf("found %d of the 2 functions", seen)
	}
}
