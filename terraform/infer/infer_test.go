package infer_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/provider/aws"
	"github.com/O6lvl4/archgopher/terraform/eval"
	"github.com/O6lvl4/archgopher/terraform/infer"
	"github.com/O6lvl4/archgopher/terraform/merge"
)

func build(t *testing.T, dir string, opt eval.Options) (model.Spec, []string) {
	t.Helper()
	ev, err := eval.Evaluate(dir, opt)
	if err != nil {
		t.Fatal(err)
	}
	return infer.Build(ev, aws.TerraformRules(), "test")
}

// edges lists each operation of each edge as from>to:kind.
func edges(s model.Spec) []string {
	var out []string
	for _, e := range s.Edges {
		for _, op := range e.Operations() {
			k := op.Kind
			if k == "" {
				k = "-"
			}
			out = append(out, e.From+">"+e.To+":"+k)
		}
	}
	sort.Strings(out)
	return out
}

func nodeByID(s model.Spec, id string) model.Node {
	for _, n := range s.Nodes {
		if n.ID == id {
			return n
		}
	}
	return model.Node{}
}

func TestExampleGraph(t *testing.T) {
	spec, warnings := build(t, "../../examples/serverless-api", eval.Options{})
	if len(warnings) > 0 {
		t.Errorf("warnings: %v", warnings)
	}
	if spec.Region != "ap-northeast-1" {
		t.Errorf("region %q", spec.Region)
	}
	want := []string{
		"api>api_handler:-",
		"api_handler>exports-sqs:send",
		"api_handler>notes:read",
		"api_handler>notes:write",
		"cleanup-lambda>notes:read",
		"cleanup-lambda>notes:write",
		"cleanup>cleanup-lambda:-",
		"exporter>exports:write",
		"exports-sqs>exporter:-",
		"users>web:-",
		"web>api:-",
		"web>assets:-",
	}
	if got := edges(spec); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("edges:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	h := nodeByID(spec, "api_handler")
	if h.Address != "module.api_handler.aws_lambda_function.this" || h.Attributes["memory_size"] != 512 {
		t.Errorf("api_handler: %+v", h)
	}
	if _, ok := h.Assumptions["durationMs"]; !ok {
		t.Error("required assumptions should be listed as placeholders")
	}
	if c := nodeByID(spec, "cleanup"); c.Load != nil || c.Traffic == nil || c.Traffic.Schedule != "rate(1 hour)" {
		t.Errorf("the schedule should become the node's traffic, as written: %+v %+v", c.Load, c.Traffic)
	}
}

func TestCountZeroRemovesResourcesAndModules(t *testing.T) {
	spec, warnings := build(t, "../../examples/serverless-api", eval.Options{Vars: map[string]string{"enable_cleanup": "false"}})
	for _, n := range spec.Nodes {
		if strings.Contains(n.Address, "cleanup") {
			t.Errorf("%s should be gone when enable_cleanup = false", n.Address)
		}
	}
	w := strings.Join(warnings, "\n")
	for _, addr := range []string{"aws_cloudwatch_event_rule.cleanup", "module.cleanup.aws_lambda_function.this"} {
		if !strings.Contains(w, addr) {
			t.Errorf("the warning should list %s as off: %q", addr, w)
		}
	}
}

func TestReferencesToCloudFrontAreMentions(t *testing.T) {
	spec, _ := build(t, "testdata/mentions", eval.Options{})
	if got := strings.Join(edges(spec), " "); got != "users>users-cognito:- users>web:-" {
		t.Errorf("a link to the site in an email is not a call: %s", got)
	}
}

func TestDataSourcesAreNotNodes(t *testing.T) {
	spec, _ := build(t, "testdata/datasources", eval.Options{})
	var ids []string
	for _, n := range spec.Nodes {
		ids = append(ids, n.ID)
	}
	sort.Strings(ids)
	if got := strings.Join(ids, " "); got != "fn own" {
		t.Errorf("a table read with a data source is managed elsewhere, want only owned nodes: %s", got)
	}
	if got := strings.Join(edges(spec), " "); got != "fn>own:read" {
		t.Errorf("edges: %s", got)
	}
}

func TestPassiveResourcesMakeNoEdges(t *testing.T) {
	spec, _ := build(t, "testdata/passive", eval.Options{})
	if got := strings.Join(edges(spec), " "); got != "" {
		t.Errorf("an alarm watches a function, it does not call it: %s", got)
	}
}

func TestPoliciesForEachAndFunctions(t *testing.T) {
	spec, warnings := build(t, "testdata/policies", eval.Options{})
	if len(warnings) > 0 {
		t.Errorf("warnings: %v", warnings)
	}
	want := []string{
		"nightly>worker:-",
		"users>worker:-",
		"worker>logs:read",
		"worker>t:read",
	}
	if got := edges(spec); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("edges:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if n := nodeByID(spec, "t"); !strings.Contains(n.Note, "2 instances") {
		t.Errorf("for_each over two tables should be noted: %q", n.Note)
	}
	if n := nodeByID(spec, "nightly"); n.Traffic == nil || n.Traffic.Schedule == "" {
		t.Errorf("a cron schedule should become the node's traffic: %+v", n.Traffic)
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
	got := strings.Join(edges(merged), " ")
	for _, e := range []string{"handler>model:-", "handler>notes:read", "api>handler:-"} {
		if !strings.Contains(got, e) {
			t.Errorf("missing edge %s in %s", e, got)
		}
	}
	again, _ := merge.Merge(merged, fresh)
	if len(again.Nodes) != len(merged.Nodes) || len(again.Edges) != len(merged.Edges) {
		t.Error("merging twice must not add anything")
	}
}

func TestNodesSitInTheirVPC(t *testing.T) {
	spec, _ := build(t, "testdata/boundaries", eval.Options{})
	var groups []string
	for _, g := range spec.Groups {
		groups = append(groups, g.ID+"="+g.Kind+":"+g.Label)
	}
	sort.Strings(groups)
	if got := strings.Join(groups, " "); got != "own=VPC:own shared=VPC:shared" {
		t.Errorf("one group per VPC, lookups of the same VPC agreeing: %s", got)
	}
	var placed []string
	for _, n := range spec.Nodes {
		placed = append(placed, n.ID+"@"+n.Group)
	}
	sort.Strings(placed)
	want := "db@shared flow@ inside@own outside@ s3@shared"
	if got := strings.Join(placed, " "); got != want {
		t.Errorf("placement:\n got %s\nwant %s", got, want)
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

func TestContainers(t *testing.T) {
	spec, _ := build(t, "testdata/containers", eval.Options{})
	want := []string{"app-ecs>orders:-", "users>app:-"}
	if got := edges(spec); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("edges: %v, want %v", got, want)
	}
	s := nodeByID(spec, "app")
	if s.Type != "aws_ecs_service" {
		t.Fatalf("app is %q", s.Type)
	}
	for k, want := range map[string]any{"cpu": 512, "memory": 1024, "cpu_architecture": "ARM64", "capacity_provider": "FARGATE", "desired_count": 2} {
		if got := s.Attributes[k]; got != want {
			t.Errorf("service %s: got %v (%T), want %v", k, got, got, want)
		}
	}
}
