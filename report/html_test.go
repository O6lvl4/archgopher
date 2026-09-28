package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/O6lvl4/archgopher/engine"
)

func TestHTMLHoldsTheDiagramAndTheTables(t *testing.T) {
	r := engine.Result{Name: "notes", Region: "ap-northeast-1", MonthlyUSD: 12.5, Nodes: []engine.NodeResult{{ID: "api", Type: "aws_apigatewayv2_api", Label: "API Gateway (HTTP)", MonthlyUSD: 12.5}}}
	var b bytes.Buffer
	if err := HTML(&b, r, []byte("<?xml version=\"1.0\"?>\n<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>\n")); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{"<!doctype html>", "<meta charset=\"utf-8\">", "<title>notes</title>", "<figure class=\"diagram\">\n<svg", "<h2>Nodes</h2>", "<code>api</code>", "$12.50"} {
		if !strings.Contains(out, want) {
			t.Errorf("html lacks %q", want)
		}
	}
	if strings.Contains(out, "<?xml") {
		t.Error("the XML declaration should not be inside the page")
	}
}
