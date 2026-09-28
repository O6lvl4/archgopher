package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// Every bundled example must read cleanly: no node errors, skips or warnings.
func TestExamplesReadCleanly(t *testing.T) {
	for _, path := range []string{"../../examples/serverless-api/notes.scouter.yaml", "../../examples/patterns/orders.scouter.yaml", "../../examples/private-network/reports.scouter.yaml", "../../examples/pay-per-use/habits.scouter.yaml"} {
		var out bytes.Buffer
		if err := run([]string{"scout", path}, &out); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		md := out.String()
		if !strings.Contains(md, "Monthly cost: **$") {
			t.Fatalf("%s: no total in:\n%s", path, md)
		}
		if strings.Contains(md, "## Problems") {
			t.Fatalf("%s has problems:\n%s", path, md[strings.Index(md, "## Problems"):])
		}
	}
}

func TestJSONAndCatalog(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"scout", "--json", "../../examples/serverless-api/notes.scouter.yaml"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"monthlyUsd"`) {
		t.Fatal("JSON output lacks monthlyUsd")
	}
	out.Reset()
	if err := run([]string{"catalog"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"aws_lambda_function"`) || !strings.Contains(out.String(), `"durationMs"`) {
		t.Fatal("catalog lacks the Lambda scouter and its fields")
	}
}

func TestExportDrawsAnSVG(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"export", "../../examples/private-network/reports.scouter.yaml"}, &out); err != nil {
		t.Fatal(err)
	}
	svg := out.String()
	for _, want := range []string{`<?xml version="1.0" encoding="UTF-8"?>`, "<svg ", `<symbol id="i-aws-lambda"`, "app-vpc", "Region ap-northeast-1", "</svg>"} {
		if !strings.Contains(svg, want) {
			t.Fatalf("export lacks %s", want)
		}
	}
	out.Reset()
	if err := run([]string{"export", "--format", "png", "../../examples/private-network/reports.scouter.yaml"}, &out); err == nil {
		t.Fatal("an unknown format should fail")
	}
}

func TestTerraformMergeIsAFixedPoint(t *testing.T) {
	var out bytes.Buffer
	spec := "../../examples/serverless-api/notes.scouter.yaml"
	if err := run([]string{"tf", "../../examples/serverless-api", "--merge", spec}, &out); err != nil {
		t.Fatal(err)
	}
	want, _ := readFile(spec)
	if out.String() != want {
		t.Fatal("tf --merge on the example changed it; regenerate the example or fix the merge")
	}
}

func readFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	return string(b), err
}
