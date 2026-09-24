package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// The bundled example must read cleanly: no node errors, skips or warnings.
func TestExampleReadsCleanly(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"scout", "../../examples/serverless-api/notes.scouter.yaml"}, &out); err != nil {
		t.Fatal(err)
	}
	md := out.String()
	if !strings.Contains(md, "Monthly cost: **$") {
		t.Fatalf("no total in:\n%s", md)
	}
	if strings.Contains(md, "## Problems") {
		t.Fatalf("the example has problems:\n%s", md[strings.Index(md, "## Problems"):])
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
