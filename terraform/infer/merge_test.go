package infer_test

import (
	"strings"
	"testing"

	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/terraform/eval"
	"github.com/O6lvl4/archgopher/terraform/merge"
)

// hasEdges fails unless every one of want is among the edges of s.
func hasEdges(t *testing.T, s model.Spec, want ...string) {
	t.Helper()
	got := strings.Join(edges(s), " ")
	for _, e := range want {
		if !strings.Contains(got, e) {
			t.Errorf("missing edge %s in %s", e, got)
		}
	}
}

func TestMergeKeepsPeoplesWork(t *testing.T) {
	fresh, _ := build(t, "../../examples/serverless-api", eval.Options{})
	existing := model.Spec{Name: "mine", Region: "ap-northeast-1",
		Nodes: []model.Node{
			{ID: "handler", Type: "aws_lambda_function", Address: "module.api_handler.aws_lambda_function.this",
				Assumptions: map[string]any{"durationMs": 80}, Note: "measured"},
			{ID: "model", Type: "bedrock_model"},
			{ID: "gone", Type: "aws_sqs_queue", Address: "aws_sqs_queue.removed"},
		},
		Edges: []model.Edge{{From: "handler", To: "model"}},
	}
	merged, warnings := merge.Merge(existing, fresh)
	h := nodeByID(merged, "handler")
	if h.Assumptions["durationMs"] != 80 || h.Note != "measured" || h.Attributes["memory_size"] != 512 {
		t.Errorf("handler: %+v", h)
	}
	if nodeByID(merged, "api_handler").ID != "" {
		t.Error("the matched node must keep its id")
	}
	if !nodeByID(merged, "gone").Stale || len(warnings) == 0 {
		t.Error("a node that left Terraform is kept, marked stale and reported")
	}
	hasEdges(t, merged, "handler>model:-", "handler>notes:read", "api>handler:-")
	again, _ := merge.Merge(merged, fresh)
	if len(again.Nodes) != len(merged.Nodes) || len(again.Edges) != len(merged.Edges) {
		t.Error("merging twice must not add anything")
	}
}

func TestMergeFollowsTerraformGroups(t *testing.T) {
	fresh, _ := build(t, "testdata/boundaries", eval.Options{})
	// A group of someone else's with the id Terraform would use, and a node
	// placed by hand in a group Terraform no longer names.
	existing := model.Spec{Name: "mine", Region: "us-east-1",
		Groups: []model.Group{{ID: "own", Kind: "Account", Label: "billing"}, {ID: "old", Kind: "VPC", Label: "old"}},
		Nodes: []model.Node{
			{ID: "fn", Type: "aws_lambda_function", Address: "aws_lambda_function.inside", Group: "old"},
			{ID: "ledger", Type: "aws_dynamodb_table", Group: "own"},
		},
	}
	merged, _ := merge.Merge(existing, fresh)
	var groups []string
	for _, g := range merged.Groups {
		groups = append(groups, g.ID+"="+g.Kind+":"+g.Label)
	}
	if got := strings.Join(groups, " "); got != "own=Account:billing own-2=VPC:own shared=VPC:shared" {
		t.Errorf("groups: %s", got)
	}
	if g := nodeByID(merged, "fn").Group; g != "own-2" {
		t.Errorf("Terraform owns where fn sits: %q", g)
	}
	if g := nodeByID(merged, "ledger").Group; g != "own" {
		t.Errorf("a node outside Terraform keeps its group: %q", g)
	}
	again, _ := merge.Merge(merged, fresh)
	if len(again.Groups) != len(merged.Groups) {
		t.Error("merging twice must not add groups")
	}
}

func TestMergeFollowsTerraformSchedules(t *testing.T) {
	fresh, _ := build(t, "../../examples/serverless-api", eval.Options{})
	addr := nodeByID(fresh, "cleanup").Address
	byHand := &model.Traffic{Rate: &model.Rate{Count: 10, Per: "day"}}
	for _, c := range []struct {
		name     string
		existing model.Node
		want     string
	}{
		{"an old schedule follows Terraform", model.Node{ID: "cleanup", Type: "aws_cloudwatch_event_rule", Address: addr, Traffic: &model.Traffic{Schedule: "rate(2 hours)"}}, "rate(1 hour)"},
		{"traffic written by hand stays", model.Node{ID: "cleanup", Type: "aws_cloudwatch_event_rule", Address: addr, Traffic: byHand}, ""},
	} {
		merged, _ := merge.Merge(model.Spec{Region: "us-east-1", Nodes: []model.Node{c.existing}}, fresh)
		got := nodeByID(merged, "cleanup").Traffic
		if c.want == "" && got != byHand || c.want != "" && (got == nil || got.Schedule != c.want) {
			t.Errorf("%s: %+v", c.name, got)
		}
	}
}
