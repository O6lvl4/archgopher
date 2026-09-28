package diagram

import (
	"os"
	"strings"
	"testing"

	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/scouter"
)

// metas is a small catalog for the tests: enough to fold, place and draw.
var metas = map[string]scouter.Meta{
	"entry":                       {Type: "entry", Label: "Entry", External: true, Icon: "general/entry"},
	"aws_apigatewayv2_api":        {Type: "aws_apigatewayv2_api", Label: "API Gateway (HTTP)", Provider: "aws", Icon: "aws/api-gateway"},
	"aws_lambda_function":         {Type: "aws_lambda_function", Label: "Lambda", Provider: "aws", Icon: "aws/lambda"},
	"aws_dynamodb_table":          {Type: "aws_dynamodb_table", Label: "DynamoDB", Provider: "aws", Icon: "aws/dynamodb"},
	"aws_rds_cluster":             {Type: "aws_rds_cluster", Label: "Aurora cluster", Provider: "aws", Icon: "aws/aurora"},
	"aws_cloudwatch_log_group":    {Type: "aws_cloudwatch_log_group", Label: "CloudWatch log group", Provider: "aws", Icon: "aws/cloudwatch", Attach: true},
	"aws_cloudwatch_metric_alarm": {Type: "aws_cloudwatch_metric_alarm", Label: "CloudWatch alarm", Provider: "aws", Icon: "aws/cloudwatch", Attach: true},
	"aws_api_gateway_stage":       {Type: "aws_api_gateway_stage", Label: "API Gateway stage", Provider: "aws", Icon: "aws/api-gateway", Attach: true},
}

func lookup(t string) (scouter.Meta, bool) {
	m, ok := metas[t]
	return m, ok
}

func TestAttachedNodesFoldIntoTheirOwnerAndEdgesReachIt(t *testing.T) {
	spec, err := model.ParseSpec([]byte(`
name: fold
region: ap-northeast-1
nodes:
  - { id: users, type: entry, load: { monthly: 1000, peakPerSecond: 1 } }
  - { id: api-stage, type: aws_api_gateway_stage }
  - { id: api, type: aws_apigatewayv2_api }
  - { id: api.handler, type: aws_lambda_function }
  - { id: api.handler-logs, type: aws_cloudwatch_log_group }
  - { id: api.errors, type: aws_cloudwatch_metric_alarm }
  - { id: api.throttles, type: aws_cloudwatch_metric_alarm }
  - { id: notes, type: aws_dynamodb_table }
edges:
  - { from: users, to: api-stage }
  - { from: api-stage, to: api }
  - { from: api, to: api.handler }
  - { from: api.handler, to: api.handler-logs, kind: ingest }
  - { from: api.handler, to: notes, kind: read, perUnit: 0.5 }
`))
	if err != nil {
		t.Fatal(err)
	}
	g, err := build(spec, lookup)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, d := range g.nodes {
		ids[d.id] = true
	}
	for _, drawn := range []string{"users", "api", "api.handler", "notes"} {
		if !ids[drawn] {
			t.Errorf("%s is not drawn", drawn)
		}
	}
	for _, folded := range []string{"api-stage", "api.handler-logs", "api.errors", "api.throttles"} {
		if ids[folded] {
			t.Errorf("%s should be folded into its owner", folded)
		}
	}
	// The stage points at the API, the log group is written by the handler,
	// and the alarms belong to the busiest node of the api module: the
	// handler, with three edges to the API's two.
	if got := g.byID["api"].attach; len(got) != 1 || got["API Gateway stage"] != 1 {
		t.Errorf("api attach = %v", got)
	}
	if got := g.byID["api.handler"].attach; got["CloudWatch log group"] != 1 || got["CloudWatch alarm"] != 2 {
		t.Errorf("handler attach = %v", got)
	}
	var seen []string
	for _, e := range g.edges {
		seen = append(seen, e.from.id+"→"+e.to.id+" "+e.label)
	}
	want := []string{"users→api ", "api→api.handler ", "api.handler→notes read ×0.5"}
	if strings.Join(seen, "|") != strings.Join(want, "|") {
		t.Errorf("edges = %v, want %v", seen, want)
	}
	if !g.byID["users"].ext {
		t.Error("the entry should sit outside the cloud")
	}
	if g.provider != "aws" {
		t.Errorf("provider = %q", g.provider)
	}
	lines := g.byID["api.handler"].lines
	if lines[0] != "Lambda" || lines[1] != "api.handler" || !strings.Contains(strings.Join(lines, "|"), "alarm ×2 · log group ×1") {
		t.Errorf("handler lines = %v", lines)
	}
}

func TestLayoutKeepsCallersLeftGroupsTightAndNothingOverlapping(t *testing.T) {
	spec, err := model.ParseSpec([]byte(`
name: layout
region: ap-northeast-1
groups:
  - { id: vpc, kind: VPC, label: app-vpc }
nodes:
  - { id: users, type: entry, load: { monthly: 1000, peakPerSecond: 1 } }
  - { id: api, type: aws_apigatewayv2_api }
  - { id: reports, type: aws_lambda_function, group: vpc }
  - { id: db, type: aws_rds_cluster, group: vpc }
  - { id: cache, type: aws_dynamodb_table }
  - { id: audit, type: aws_dynamodb_table }
edges:
  - { from: users, to: api }
  - { from: api, to: reports }
  - { from: reports, to: db, kind: query }
  - { from: api, to: cache, kind: read }
  - { from: users, to: audit, kind: write }
`))
	if err != nil {
		t.Fatal(err)
	}
	g, err := build(spec, lookup)
	if err != nil {
		t.Fatal(err)
	}
	g.layout()
	at := func(id string) *node { return g.byID[id] }
	if !(at("users").cx < at("api").cx && at("api").cx < at("reports").cx && at("reports").cx < at("db").cx) {
		t.Errorf("callers should be left of what they call: users %v api %v reports %v db %v", at("users").cx, at("api").cx, at("reports").cx, at("db").cx)
	}
	f := g.frames[0]
	for _, m := range f.members {
		if m.left() < f.x || m.right() > f.x+f.w || m.top() < f.y || m.bottom() > f.y+f.h {
			t.Errorf("%s is outside its frame", m.id)
		}
	}
	for _, d := range g.nodes {
		if d.src.Group == "" && d.cx > f.x && d.cx < f.x+f.w && d.cy > f.y && d.cy < f.y+f.h {
			t.Errorf("%s fell inside the VPC frame", d.id)
		}
	}
	for i, a := range g.nodes {
		for _, b := range g.nodes[i+1:] {
			if a.left() < b.right() && b.left() < a.right() && a.top() < b.bottom() && b.top() < a.bottom() {
				t.Errorf("%s and %s overlap", a.id, b.id)
			}
		}
	}
	if at("users").cx >= g.cloud.x {
		t.Errorf("the entry at %v should be left of the cloud frame at %v", at("users").cx, g.cloud.x)
	}
	if g.region.w == 0 || g.region.x < g.cloud.x || g.region.x+g.region.w > g.cloud.x+g.cloud.w {
		t.Errorf("region %+v should sit inside the cloud %+v", g.region, g.cloud)
	}
	// The edge from users to audit skips the api column, so it bends there.
	for _, e := range g.edges {
		if e.from.id == "users" && e.to.id == "audit" && len(e.via) == 0 {
			t.Error("users→audit skips a column and should bend through it")
		}
	}
	svg := string(g.svg())
	for _, want := range []string{`<symbol id="i-aws-lambda"`, `<use href="#i-aws-aurora"`, `VPC <tspan`, `app-vpc`, `Region ap-northeast-1`, `AWS Cloud`, `>query<`} {
		if !strings.Contains(svg, want) {
			t.Errorf("svg lacks %s", want)
		}
	}
	if strings.Count(svg, `<symbol id="i-aws-dynamodb"`) != 1 {
		t.Error("one symbol per icon")
	}
}

func TestSavedPositionsAreUsed(t *testing.T) {
	spec, err := model.ParseSpec([]byte(`
name: placed
region: ap-northeast-1
nodes:
  - { id: api, type: aws_apigatewayv2_api, position: { x: 0, y: 0 } }
  - { id: fn, type: aws_lambda_function, position: { x: 0, y: 400 } }
edges:
  - { from: api, to: fn }
`))
	if err != nil {
		t.Fatal(err)
	}
	g, err := build(spec, lookup)
	if err != nil {
		t.Fatal(err)
	}
	g.layout()
	if a, b := g.byID["api"], g.byID["fn"]; a.cx != b.cx || b.cy-a.cy != 400*positionScale {
		t.Errorf("positions not kept: api (%v,%v) fn (%v,%v)", a.cx, a.cy, b.cx, b.cy)
	}
}

func TestExamplesDraw(t *testing.T) {
	for _, path := range []string{"../examples/private-network/reports.scouter.yaml", "../examples/patterns/orders.scouter.yaml"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		spec, err := model.ParseSpec(data)
		if err != nil {
			t.Fatal(err)
		}
		out, err := SVG(spec, lookup)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if !strings.HasPrefix(string(out), "<svg ") || !strings.HasSuffix(strings.TrimSpace(string(out)), "</svg>") {
			t.Fatalf("%s: not an SVG document", path)
		}
	}
}
