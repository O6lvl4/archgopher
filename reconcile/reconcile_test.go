package reconcile

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/O6lvl4/archgopher/book"
	"github.com/O6lvl4/archgopher/engine"
	"github.com/O6lvl4/archgopher/meter"
)

const bill = `{"ResultsByTime":[{"TimePeriod":{"Start":"2026-09-01","End":"2026-10-01"},"Groups":[
 {"Keys":["Amazon Simple Storage Service"],"Metrics":{"UnblendedCost":{"Amount":"10.5","Unit":"USD"}}},
 {"Keys":["Claude Haiku 4.5 (Amazon Bedrock Edition)"],"Metrics":{"UnblendedCost":{"Amount":"20","Unit":"USD"}}},
 {"Keys":["AWS Lambda"],"Metrics":{"UnblendedCost":{"Amount":"0.5","Unit":"USD"}}},
 {"Keys":["AWS WAF"],"Metrics":{"UnblendedCost":{"Amount":"30","Unit":"USD"}}},
 {"Keys":["Tax"],"Metrics":{"UnblendedCost":{"Amount":"6.1","Unit":"USD"}}}]}]}`

func f(v float64) *float64 { return &v }

func TestCompareSplitsTheBill(t *testing.T) {
	b, err := ParseCostExplorer([]byte(bill))
	if err != nil {
		t.Fatal(err)
	}
	if b.Period != "2026-09-01..2026-10-01" || len(b.Lines) != 5 {
		t.Fatalf("bill %+v", b)
	}
	books := book.Books{Prices: book.Book{
		"aws.s3.standard.storage":             {Sync: json.RawMessage(`{"service":"AmazonS3"}`)},
		"aws.bedrock.claude-haiku-4-5.global": {Sync: json.RawMessage(`{"service":"AmazonBedrockFoundationModels"}`), Rows: map[string]map[string]*float64{"input": {}}},
		"aws.lambda.requests":                 {Sync: json.RawMessage(`{"service":"AWSLambda"}`)},
		"cf.workers.requests":                 {},
	}}
	names := Names{
		"AmazonS3":                      {"Amazon Simple Storage Service"},
		"AmazonBedrockFoundationModels": {"* (Amazon Bedrock Edition)"},
		"AWSLambda":                     {"AWS Lambda"},
		"awswaf":                        {"AWS WAF"},
	}
	res := engine.Result{Nodes: []engine.NodeResult{
		{ID: "landing", Costs: []meter.Cost{{Name: "Storage", PriceID: "aws.s3.standard.storage", MonthlyUSD: f(11)}}},
		{ID: "haiku", Costs: []meter.Cost{{Name: "Input tokens", PriceID: "aws.bedrock.claude-haiku-4-5.global.input", MonthlyUSD: f(19)}}},
		{ID: "worker", Costs: []meter.Cost{{Name: "Requests", PriceID: "cf.workers.requests", MonthlyUSD: f(5)}}},
		{ID: "all", Members: []string{"landing", "haiku"}, Costs: []meter.Cost{{Name: "Storage", PriceID: "aws.s3.standard.storage", MonthlyUSD: f(999)}}},
	}}
	r := Compare("app", res, books, names, b)
	if r.BillUSD != 67.1 || r.TaxUSD != 6.1 || r.CoveredUSD != 30.5 || r.OutsideUSD != 30.5 || r.DeclaredUSD != 35 {
		t.Errorf("split: bill %v tax %v covered %v outside %v declared %v", r.BillUSD, r.TaxUSD, r.CoveredUSD, r.OutsideUSD, r.DeclaredUSD)
	}
	if len(r.Rows) != 2 || r.Rows[0].Service != "Claude Haiku 4.5 (Amazon Bedrock Edition)" || r.Rows[0].DeclaredUSD != 19 || r.Rows[1].Nodes[0] != "landing" {
		t.Errorf("rows %+v", r.Rows)
	}
	if len(r.Outside) != 2 || r.Outside[0].Service != "AWS WAF" || r.Outside[1].Service != "AWS Lambda" {
		t.Errorf("outside: WAF has no code, Lambda has one but no node: %+v", r.Outside)
	}
	if len(r.Unmapped) != 1 || r.Unmapped[0].Node != "worker" {
		t.Errorf("a price without a service code is listed, not dropped: %+v", r.Unmapped)
	}
	var out strings.Builder
	if err := Markdown(&out, r); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"| Bill | $67.10 |", "## By service", "## Billed, no node stands for it", "| AWS WAF | $30.00 |", "## Lines without a billing service", "+14.8%"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("markdown lacks %q:\n%s", want, out.String())
		}
	}
}

func TestParseCostExplorerRejectsOtherShapes(t *testing.T) {
	if _, err := ParseCostExplorer([]byte(`{"nodes":[]}`)); err == nil || !strings.Contains(err.Error(), "SERVICE") {
		t.Errorf("a scout result is not a bill: %v", err)
	}
}
