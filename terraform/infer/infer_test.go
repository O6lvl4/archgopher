package infer_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/provider/aws"
	"github.com/O6lvl4/archgopher/terraform/eval"
	"github.com/O6lvl4/archgopher/terraform/infer"
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

func TestEdgeResources(t *testing.T) {
	spec, warnings := build(t, "testdata/edge", eval.Options{})
	if len(warnings) > 0 {
		t.Errorf("warnings: %v", warnings)
	}
	want := "prod>api:- tokyo>web:- users>edge:- users>prod:- edge>tokyo:-"
	got := edges(spec)
	sort.Strings(got)
	wantList := strings.Fields(want)
	sort.Strings(wantList)
	if strings.Join(got, " ") != strings.Join(wantList, " ") {
		t.Errorf("edges: %s\nwant: %s", strings.Join(got, " "), strings.Join(wantList, " "))
	}
	// A number that points at repeated blocks counts them; a flag that points
	// at an attribute set from a reference reads true.
	if got := nodeByID(spec, "out").Attributes["ip_address"]; got != 3.0 {
		t.Errorf("resolver endpoint ip_address: got %v, want 3", got)
	}
	if got := nodeByID(spec, "web").Attributes["subnet_mapping"]; got != 2.0 {
		t.Errorf("load balancer subnet_mapping: got %v, want 2", got)
	}
	if got := nodeByID(spec, "web").Attributes["elastic_ips"]; got != true {
		t.Errorf("load balancer on Elastic IPs: got %v", got)
	}
	if got := nodeByID(spec, "internal").Attributes["private"]; got != true {
		t.Errorf("certificate from a private CA: got %v", got)
	}
}

func TestResourcesInBetweenAreFedByTheirSource(t *testing.T) {
	spec, _ := build(t, "testdata/inbetween", eval.Options{})
	want := "archive>archive-s3:- clicks>archive:- orders-sqs>orders:- orders>enrich:- orders>handler:-"
	if got := strings.Join(edges(spec), " "); got != want {
		t.Errorf("a pipe and a delivery stream sit between their source and target:\n got %s\nwant %s", got, want)
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
	if n := nodeByID(spec, "t"); n.Instances != 2 {
		t.Errorf("for_each over two tables should make two instances: %d", n.Instances)
	}
	if n := nodeByID(spec, "nightly"); n.Traffic == nil || n.Traffic.Schedule == "" {
		t.Errorf("a cron schedule should become the node's traffic: %+v", n.Traffic)
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

// A subscription sits between its topic and its endpoint, and an event bus
// reaches the targets that name it.
func TestFanOutThroughSubscriptionsAndBuses(t *testing.T) {
	spec, _ := build(t, "testdata/fanout", eval.Options{})
	want := []string{
		"app>orders-sns:-",
		"billing>billing-sqs:-",
		"orders-sns>billing:-",
		"orders-sns>webhook:-",
		"orders>orders-sns:-",
	}
	if got := edges(spec); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("edges:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if n := nodeByID(spec, "webhook"); n.Attributes["protocol"] != "https" {
		t.Errorf("the subscription should read its protocol: %+v", n.Attributes)
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

func TestAttributesReadFromReferencesAndSettings(t *testing.T) {
	spec, _ := build(t, "testdata/settings", eval.Options{})
	attrs := map[string]map[string]any{}
	for _, n := range spec.Nodes {
		attrs[n.ID] = n.Attributes
	}
	if got := attrs["on_host"]["on_host"]; got != true {
		t.Errorf("host_id references a host: on_host = %v, want true", got)
	}
	if got := attrs["literal"]["on_host"]; got != true {
		t.Errorf("host_id is a literal id: on_host = %v, want true", got)
	}
	env := attrs["env"]
	if env["instance_type"] != "t3.small" || env["min_size"] != "2" || env["stream_logs"] != true {
		t.Errorf("settings are read by name: %v", env)
	}
	if got := strings.Join(edges(spec), " "); got != "users>env:-" {
		t.Errorf("an instance on a host does not call it: %s", got)
	}
}

func TestCountsMultiplyThroughModules(t *testing.T) {
	spec, _ := build(t, "testdata/counted", eval.Options{})
	counts := map[string]int{}
	for _, n := range spec.Nodes {
		counts[n.Address] = n.Instances
	}
	// Three queues in each of two copies of the module; a list of teams
	// that has no value yet is unknown until apply.
	if got := counts["module.region.aws_sqs_queue.work"]; got != 6 {
		t.Errorf("3 queues × 2 modules = %d instances, want 6 (%v)", got, counts)
	}
	if got := counts["aws_sqs_queue.per_team"]; got != model.UnknownInstances {
		t.Errorf("a count from an unset variable = %d, want unknown", got)
	}
}

func TestDifferentInstancesSplitAndAlikeOnesStay(t *testing.T) {
	spec, _ := build(t, "testdata/variants", eval.Options{})
	got := map[string]string{}
	for _, n := range spec.Nodes {
		got[n.ID] = fmt.Sprintf("%v ×%d %s", n.Attributes["instance_type"], n.Instances, n.Address)
	}
	want := map[string]string{
		"app-batch": `m5.large ×0 aws_instance.app["batch"]`,
		"app-web":   `t3.micro ×2 aws_instance.app["web"]`,
		"work":      "<nil> ×3 aws_sqs_queue.work",
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("%s = %q, want %q (all: %v)", id, got[id], w, got)
		}
	}
}
