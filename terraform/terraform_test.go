package terraform_test

import (
	"math"
	"sort"
	"strings"
	"testing"

	"github.com/O6lvl4/arch-scouter/aws"
	"github.com/O6lvl4/arch-scouter/scout"
	"github.com/O6lvl4/arch-scouter/terraform"
)

func build(t *testing.T, dir string, opt terraform.Options) (scout.Spec, []string) {
	t.Helper()
	ev, err := terraform.Evaluate(dir, opt)
	if err != nil {
		t.Fatal(err)
	}
	return terraform.Build(ev, aws.TerraformRules(), "test")
}

func edges(s scout.Spec) []string {
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

func nodeByID(s scout.Spec, id string) scout.Node {
	for _, n := range s.Nodes {
		if n.ID == id {
			return n
		}
	}
	return scout.Node{}
}

func TestExampleGraph(t *testing.T) {
	spec, warnings := build(t, "../examples/serverless-api", terraform.Options{})
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
	spec, warnings := build(t, "../examples/serverless-api", terraform.Options{Vars: map[string]string{"enable_cleanup": "false"}})
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
	spec, _ := build(t, "testdata/mentions", terraform.Options{})
	if got := strings.Join(edges(spec), " "); got != "users>users-cognito:- users>web:-" {
		t.Errorf("a link to the site in an email is not a call: %s", got)
	}
}

func TestPoliciesForEachAndFunctions(t *testing.T) {
	spec, warnings := build(t, "testdata/policies", terraform.Options{})
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
	fresh, _ := build(t, "../examples/serverless-api", terraform.Options{})
	existing := scout.Spec{Name: "mine", Region: "ap-northeast-1",
		Nodes: []scout.Node{
			{ID: "handler", Type: "aws_lambda_function", Address: "module.api_handler.aws_lambda_function.this",
				Assumptions: map[string]any{"durationMs": 80}, Note: "measured"},
			{ID: "model", Type: "bedrock_model"},
			{ID: "gone", Type: "aws_sqs_queue", Address: "aws_sqs_queue.removed"},
		},
		Edges: []scout.Edge{{From: "handler", To: "model"}},
	}
	merged, warnings := terraform.Merge(existing, fresh)
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
	again, _ := terraform.Merge(merged, fresh)
	if len(again.Nodes) != len(merged.Nodes) || len(again.Edges) != len(merged.Edges) {
		t.Error("merging twice must not add anything")
	}
}

func TestScheduleLoad(t *testing.T) {
	cases := map[string]float64{
		"rate(5 minutes)":         float64(scout.SecondsPerMonth) / 300,
		"rate(1 day)":             float64(scout.SecondsPerMonth) / 86400,
		"cron(0 9 * * ? *)":       30.4,
		"cron(0/15 * * * ? *)":    4 * 24 * 30.4,
		"cron(0 9 ? * MON-FRI *)": 30.4 * 1 / 7, // named days are not parsed: counted as one day a week
		"cron(0 9 1 * ? *)":       1,
	}
	for expr, want := range cases {
		got, err := terraform.ScheduleLoad(expr)
		if err != nil {
			t.Errorf("%s: %v", expr, err)
			continue
		}
		if math.Abs(got.Monthly-want) > 1e-6 {
			t.Errorf("%s: got %v, want %v", expr, got.Monthly, want)
		}
	}
	if _, err := terraform.ScheduleLoad("at(2026-01-01T00:00:00)"); err == nil {
		t.Error("one-off schedules have no steady load")
	}
}
