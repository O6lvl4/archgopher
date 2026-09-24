package infer_test

import (
	"math"
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

func edges(s model.Spec) []string {
	var out []string
	for _, e := range s.Edges {
		k := e.Kind
		if k == "" {
			k = "-"
		}
		out = append(out, e.From+">"+e.To+":"+k)
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
	if c := nodeByID(spec, "cleanup"); c.Load == nil || c.Load.Monthly != 730 {
		t.Errorf("rate(1 hour) should become 730 fires a month: %+v", c.Load)
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
	if n := nodeByID(spec, "nightly"); n.Load == nil || math.Abs(n.Load.Monthly-30.4) > 1e-9 {
		t.Errorf("a daily cron fires about 30.4 times a month: %+v", n.Load)
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
