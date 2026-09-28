package main

import (
	"bytes"
	"os"
	"path/filepath"
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

func TestExportDrawsSVGPNGAndHTML(t *testing.T) {
	spec := "../../examples/private-network/reports.scouter.yaml"
	var out bytes.Buffer
	if err := run([]string{"export", spec}, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<?xml version="1.0" encoding="UTF-8"?>`, "<svg ", `<symbol id="i-aws-lambda"`, "app-vpc", "Region ap-northeast-1", "</svg>"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("svg lacks %s", want)
		}
	}
	dir := t.TempDir()
	png := filepath.Join(dir, "reports.png")
	if err := run([]string{"export", spec, "-o", png}, &out); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(png); err != nil || !bytes.HasPrefix(b, []byte("\x89PNG")) {
		t.Fatalf("-o reports.png should write a PNG: %v", err)
	}
	out.Reset()
	if err := run([]string{"export", "--format", "html", spec}, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<!doctype html>", "<svg ", "<h2>Nodes</h2>", "<code>reports</code>"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("html lacks %s", want)
		}
	}
	if err := run([]string{"export", "--format", "pdf", spec}, &out); err == nil || !strings.Contains(err.Error(), "svg, png, html") {
		t.Fatalf("an unknown format should name the formats: %v", err)
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
