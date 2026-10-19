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

func TestUsageTypesSharedByModelEditionsAreNarrowedByThePrice(t *testing.T) {
	bill := Bill{Usage: []UsageLine{
		{Service: "Claude Haiku 4.5 (Amazon Bedrock Edition)", UsageType: "APN1-MP:APN1_InputTokenCount-Units", USD: 17, Quantity: 16, Unit: "1M tokens"},
		{Service: "Claude Sonnet 4.6 (Amazon Bedrock Edition)", UsageType: "APN1-MP:APN1_InputTokenCount-Units", USD: 1, Quantity: 0.3, Unit: "1M tokens"},
	}}
	books := book.Books{Prices: book.Book{
		"aws.bedrock.claude-haiku-4-5.regional.input": {Sync: json.RawMessage(`{"service":"AmazonBedrockFoundationModels","filters":{"usagetype":".*InputTokenCount-Units"}}`)},
	}}
	names := Names{"AmazonBedrockFoundationModels": {"* (Amazon Bedrock Edition)"}}
	res := engine.Result{Nodes: []engine.NodeResult{{ID: "haiku", Costs: []meter.Cost{{Name: "Input tokens", PriceID: "aws.bedrock.claude-haiku-4-5.regional.input", Quantity: 16e6, Unit: "token", MonthlyUSD: f(17.2)}}}}}
	rows, unread := compareUsage(res, books, names, bill, map[string]map[string]bool{"AmazonBedrockFoundationModels": {"haiku": true}})
	if len(rows) != 1 || rows[0].Service != "Claude Haiku 4.5 (Amazon Bedrock Edition)" || rows[0].BillUSD != 17 || len(rows[0].UsageTypes) != 1 {
		t.Errorf("the Haiku price should be read against the Haiku edition only: %+v", rows)
	}
	if len(unread) != 1 || unread[0].Service != "Claude Sonnet 4.6 (Amazon Bedrock Edition)" {
		t.Errorf("the Sonnet edition has no price reading it: %+v", unread)
	}
}

func TestParseCostExplorerRejectsOtherShapes(t *testing.T) {
	if _, err := ParseCostExplorer([]byte(`{"nodes":[]}`)); err == nil || !strings.Contains(err.Error(), "SERVICE") {
		t.Errorf("a scout result is not a bill: %v", err)
	}
}

const usageBill = `{"GroupDefinitions":[{"Type":"DIMENSION","Key":"SERVICE"},{"Type":"DIMENSION","Key":"USAGE_TYPE"}],"ResultsByTime":[{"TimePeriod":{"Start":"2026-09-01","End":"2026-10-01"},"Groups":[
 {"Keys":["Amazon Simple Storage Service","APN1-TimedStorage-ByteHrs"],"Metrics":{"UnblendedCost":{"Amount":"6.0","Unit":"USD"},"UsageQuantity":{"Amount":"254","Unit":"GB-Mo"}}},
 {"Keys":["Amazon Simple Storage Service","APN1-Requests-Tier2"],"Metrics":{"UnblendedCost":{"Amount":"0.06","Unit":"USD"},"UsageQuantity":{"Amount":"168337","Unit":"Requests"}}},
 {"Keys":["Amazon Simple Storage Service","APN1-DataTransfer-Out-Bytes"],"Metrics":{"UnblendedCost":{"Amount":"4.5","Unit":"USD"},"UsageQuantity":{"Amount":"41.3","Unit":"GB"}}},
 {"Keys":["AWS WAF","APN1-WebACLV2"],"Metrics":{"UnblendedCost":{"Amount":"5","Unit":"USD"},"UsageQuantity":{"Amount":"1","Unit":"Months"}}}]}]}`

func TestCompareByUsageType(t *testing.T) {
	b, err := ParseCostExplorer([]byte(usageBill))
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Lines) != 2 || len(b.Usage) != 4 || b.Lines[0].USD < 10.559 || b.Lines[0].USD > 10.561 {
		t.Fatalf("bill %+v", b)
	}
	books := book.Books{Prices: book.Book{
		"aws.s3.standard.storage": {Sync: json.RawMessage(`{"service":"AmazonS3","filters":{"usagetype":"((?:[A-Z]+[0-9]|EU)-)?TimedStorage-ByteHrs"}}`)},
		"aws.s3.standard.get":     {Sync: json.RawMessage(`{"service":"AmazonS3","filters":{"usagetype":"((?:[A-Z]+[0-9]|EU)-)?Requests-Tier2"}}`)},
	}}
	names := Names{"AmazonS3": {"Amazon Simple Storage Service"}, "awswaf": {"AWS WAF"}}
	res := engine.Result{Nodes: []engine.NodeResult{
		{ID: "landing", Costs: []meter.Cost{
			{Name: "Storage", PriceID: "aws.s3.standard.storage", Quantity: 254, Unit: "GB-month", MonthlyUSD: f(6.35)},
			{Name: "GET requests", PriceID: "aws.s3.standard.get", Quantity: 77731, Unit: "request", MonthlyUSD: f(0.03)},
		}},
		{ID: "spa", Costs: []meter.Cost{{Name: "Storage", PriceID: "aws.s3.standard.storage", Quantity: 1, Unit: "GB-month", MonthlyUSD: f(0.03)}}},
	}}
	r := Compare("app", res, books, names, b)
	if len(r.UsageRows) != 2 {
		t.Fatalf("usage rows %+v", r.UsageRows)
	}
	st := r.UsageRows[0]
	if st.PriceID != "aws.s3.standard.storage" || st.BillQty != 254 || st.BillUnit != "GB-Mo" || st.DeclaredQty != 255 || st.DeclaredUSD != 6.38 || len(st.Lines) != 2 {
		t.Errorf("storage row should gather both buckets against the one usage type: %+v", st)
	}
	if len(r.UnreadUsage) != 1 || r.UnreadUsage[0].UsageType != "APN1-DataTransfer-Out-Bytes" {
		t.Errorf("S3 egress is billed and no line reads it; WAF is outside the declaration and not listed here: %+v", r.UnreadUsage)
	}
	var out strings.Builder
	if err := Markdown(&out, r); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## By usage type", "| 254.00 GB-Mo | 255.00 GB-month |", "## Billed usage no line reads", "| APN1-DataTransfer-Out-Bytes | $4.50 | 41.30 GB |"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("markdown lacks %q:\n%s", want, out.String())
		}
	}
}
