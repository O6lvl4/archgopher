package pattern_test

import (
	"math"
	"strings"
	"testing"

	"github.com/O6lvl4/archgopher/engine"
	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/pattern"
	"github.com/O6lvl4/archgopher/provider/aws"
	awspattern "github.com/O6lvl4/archgopher/provider/aws/pattern"
	"github.com/O6lvl4/archgopher/scouter"
)

func run(t *testing.T, spec model.Spec) (engine.Result, pattern.Expansion) {
	t.Helper()
	reg := awspattern.Registry()
	expanded, exp, err := pattern.Expand(spec, reg)
	if err != nil {
		t.Fatal(err)
	}
	books, err := aws.Books()
	if err != nil {
		t.Fatal(err)
	}
	res, err := engine.Run(expanded, aws.Registry(), books)
	if err != nil {
		t.Fatal(err)
	}
	return pattern.Rollup(res, spec, exp, reg), exp
}

// A pattern placed as one node reads the same as the nodes written by hand.
func TestPatternEqualsItsExpansionWrittenByHand(t *testing.T) {
	load := &model.Load{Monthly: 1e6, PeakPerSecond: 20}
	placed := model.Spec{Region: "us-east-1", Nodes: []model.Node{
		{ID: "users", Type: scouter.EntryType, Load: load},
		{ID: "orders", Type: "aws.pattern.serverless_api", Assumptions: map[string]any{"durationMs": 100, "itemSizeKb": 2, "storageGb": 10}},
	}, Edges: []model.Edge{{From: "users", To: "orders"}}}
	res, exp := run(t, placed)

	one, perWrite := 1.0, 0.2
	byHand := model.Spec{Region: "us-east-1", Nodes: []model.Node{
		{ID: "users", Type: scouter.EntryType, Load: load},
		{ID: "api", Type: "aws_api_gateway_rest_api"},
		{ID: "fn", Type: "aws_lambda_function", Attributes: map[string]any{"memory_size": 512.0, "architectures": []any{"arm64"}}, Assumptions: map[string]any{"durationMs": 100}},
		{ID: "table", Type: "aws_dynamodb_table", Attributes: map[string]any{"billing_mode": "PAY_PER_REQUEST"}, Assumptions: map[string]any{"itemSizeKb": 2, "storageGb": 10}},
	}, Edges: []model.Edge{
		{From: "users", To: "api"}, {From: "api", To: "fn"},
		{From: "fn", To: "table", Kind: "read", PerUnit: &one}, {From: "fn", To: "table", Kind: "write", PerUnit: &perWrite},
	}}
	want, _ := run(t, byHand)
	if math.Abs(res.MonthlyUSD-want.MonthlyUSD) > 1e-9 || res.MonthlyUSD == 0 {
		t.Fatalf("placed %v, by hand %v", res.MonthlyUSD, want.MonthlyUSD)
	}
	if got := strings.Join(exp["orders"].Members, ","); got != "orders/api,orders/fn,orders/table" {
		t.Fatalf("members: %s", got)
	}
	g := res.Nodes[len(res.Nodes)-1]
	if g.ID != "orders" || math.Abs(g.MonthlyUSD-res.MonthlyUSD) > 1e-9 || len(g.Limits) == 0 {
		t.Fatalf("rollup: %+v", g)
	}
}

func TestEdgesLeaveFromTheOutlet(t *testing.T) {
	spec := model.Spec{Region: "us-east-1", Nodes: []model.Node{
		{ID: "u", Type: scouter.EntryType, Load: &model.Load{Monthly: 1000, PeakPerSecond: 1}},
		{ID: "jobs", Type: "aws.pattern.queue_worker", Assumptions: map[string]any{"durationMs": 2000, "batchSize": 10}},
		{ID: "out", Type: "aws_s3_bucket", Assumptions: map[string]any{"storageGb": 1}},
	}, Edges: []model.Edge{{From: "u", To: "jobs"}, {From: "jobs", To: "out", Kind: "write"}}}
	res, _ := run(t, spec)
	for _, n := range res.Nodes {
		if n.ID == "out" && n.Demand["write"].Monthly != 100 {
			t.Fatalf("1,000 messages in batches of 10 write 100 times, got %v", n.Demand["write"].Monthly)
		}
	}
}

func TestMissingParametersAreErrors(t *testing.T) {
	spec := model.Spec{Region: "us-east-1", Nodes: []model.Node{{ID: "p", Type: "aws.pattern.serverless_api"}}}
	if _, _, err := pattern.Expand(spec, awspattern.Registry()); err == nil || !strings.Contains(err.Error(), `missing parameter "durationMs"`) {
		t.Fatalf("want a missing parameter, got %v", err)
	}
}

func TestPatternKeepsItsGroup(t *testing.T) {
	spec := model.Spec{Region: "us-east-1", Groups: []model.Group{{ID: "vpc", Kind: "VPC"}}, Nodes: []model.Node{
		{ID: "u", Type: scouter.EntryType, Load: &model.Load{Monthly: 1000, PeakPerSecond: 1}},
		{ID: "jobs", Type: "aws.pattern.queue_worker", Group: "vpc", Assumptions: map[string]any{"durationMs": 2000}},
	}, Edges: []model.Edge{{From: "u", To: "jobs"}}}
	expanded, _, err := pattern.Expand(spec, awspattern.Registry())
	if err != nil {
		t.Fatal(err)
	}
	if len(expanded.Groups) != 1 {
		t.Fatalf("groups: %+v", expanded.Groups)
	}
	for _, n := range expanded.Nodes {
		if n.ID != "u" && n.Group != "vpc" {
			t.Errorf("%s sits in %q, want the pattern's group", n.ID, n.Group)
		}
	}
}
