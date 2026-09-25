package api

import (
	"math"

	"encoding/json"
	"github.com/O6lvl4/archgopher/engine"
	"github.com/O6lvl4/archgopher/model"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// files reads a directory tree into the map the browser sends.
func files(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		rel, _ := filepath.Rel(root, p)
		out["project/"+filepath.ToSlash(rel)] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTerraformInMemoryMatchesTheCLI(t *testing.T) {
	fs := files(t, "../examples/serverless-api")
	if roots := RootCandidates(fs); strings.Join(roots, ",") != "project,project/modules/function" {
		t.Fatalf("roots: %v", roots)
	}
	yaml, err := os.ReadFile("../examples/serverless-api/notes.scouter.yaml")
	if err != nil {
		t.Fatal(err)
	}
	existing, err := ParseYAML(string(yaml))
	if err != nil {
		t.Fatal(err)
	}
	res, err := Terraform(TerraformRequest{Files: fs, Root: "project", Merge: &existing})
	if err != nil {
		t.Fatal(err)
	}
	got, err := MarshalYAML(res.Spec)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(yaml) {
		t.Fatal("merging the in-memory tree into the example must be a fixed point, like the CLI")
	}
}

func TestModulesOutsideTheFilesAreReported(t *testing.T) {
	res, err := Terraform(TerraformRequest{Files: map[string]string{"env/main.tf": `module "x" { source = "../../elsewhere" }`}, Root: "env"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(res.Warnings, " "), "outside the files given") {
		t.Fatalf("warnings: %v", res.Warnings)
	}
}

func TestCallEnvelope(t *testing.T) {
	var ok struct {
		Ok []CatalogEntry `json:"ok"`
	}
	if err := json.Unmarshal([]byte(Call("catalog", "")), &ok); err != nil || len(ok.Ok) == 0 {
		t.Fatalf("catalog: %v", err)
	}
	var bad struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(Call("scout", `{"region":"us-east-1","nodes":[{"id":"a","type":"entry"}],"edges":[{"from":"a","to":"b"}]}`)), &bad); err != nil || !strings.Contains(bad.Error, `no node "b"`) {
		t.Fatalf("structural errors come back as an error envelope: %+v %v", bad, err)
	}
}

// Every node the UI can place has a picture, and every picture is used.
func TestEveryEntryHasAnIcon(t *testing.T) {
	dir := filepath.Join("..", "web", "src", "ui", "icons")
	used := map[string]bool{}
	for _, e := range Catalog() {
		if e.Icon == "" {
			t.Errorf("%s: no icon", e.Type)
			continue
		}
		p := filepath.Join(dir, filepath.FromSlash(e.Icon)+".svg")
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s: icon %q: %v", e.Type, e.Icon, err)
		}
		used[filepath.Clean(p)] = true
	}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".svg") && !used[filepath.Clean(p)] {
			t.Errorf("%s: no node uses it", p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Traffic between two nodes of one VPC is read by the VPC: 10 million calls of
// 30 KB over three zones cross 2/3 of 286.1 GB, charged out and in.
func TestZoneCrossingsAreReadByTheVPC(t *testing.T) {
	kb := 30.0
	spec := model.Spec{Region: "ap-northeast-1",
		Groups: []model.Group{{ID: "main", Kind: "VPC", Type: "aws_vpc", Assumptions: map[string]any{"zones": 3}}},
		Nodes: []model.Node{
			{ID: "u", Type: "entry", Load: &model.Load{Monthly: 1e7, PeakPerSecond: 10}},
			{ID: "fn", Type: "aws_lambda_function", Group: "main", Assumptions: map[string]any{"durationMs": 50}},
			{ID: "db", Type: "aws_rds_cluster", Group: "main", Assumptions: map[string]any{"averageAcu": 1, "storageGb": 1}},
			{ID: "table", Type: "aws_dynamodb_table", Attributes: map[string]any{"billing_mode": "PAY_PER_REQUEST"},
				Assumptions: map[string]any{"itemSizeKb": 1, "storageGb": 1}},
		},
		Edges: []model.Edge{{From: "u", To: "fn"}, {From: "fn", To: "db", KB: &kb}, {From: "fn", To: "table", Kind: "read", KB: &kb}},
	}
	res, err := Scout(spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Groups) != 1 {
		t.Fatalf("groups: %+v", res.Groups)
	}
	crossing := 1e7 * kb / 1024 / 1024 * 2 / 3
	if got, want := res.Groups[0].MonthlyUSD, crossing*0.01*2; math.Abs(got-want) > 1e-9 {
		t.Errorf("VPC reads $%v, want $%v", got, want)
	}
	// The same kb sizes the table's reads: 30 KB is eight 4 KB units, half
	// each when eventually consistent.
	if got, want := readUnits(t, res, "table"), 1e7*8*0.5; math.Abs(got-want) > 1e-6 {
		t.Errorf("table reads %v units, want %v", got, want)
	}
}

func readUnits(t *testing.T, res engine.Result, id string) float64 {
	t.Helper()
	for _, n := range res.Nodes {
		if n.ID != id {
			continue
		}
		if n.Error != "" {
			t.Fatalf("%s: %s", id, n.Error)
		}
		for _, c := range n.Costs {
			if c.Name == "Read request units" {
				return c.Quantity
			}
		}
	}
	t.Fatalf("%s has no read units", id)
	return 0
}

// One call can do several kinds of work, each of its own size: per call, a
// 25 KB Query (7 units), two 1 KB GetItems (1 unit each) and, one call in
// ten, a 2 KB write (2 units). The table's item size is never used.
func TestOperationsOfOneEdge(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	spec := model.Spec{Region: "us-east-1",
		Nodes: []model.Node{
			{ID: "u", Type: "entry", Load: &model.Load{Monthly: 1e6, PeakPerSecond: 10}},
			{ID: "table", Type: "aws_dynamodb_table", Attributes: map[string]any{"billing_mode": "PAY_PER_REQUEST"},
				Assumptions: map[string]any{"storageGb": 1, "consistentRead": "strong"}},
		},
		Edges: []model.Edge{{From: "u", To: "table", Ops: []model.Op{
			{Kind: "read", KB: f(25)},
			{Kind: "read", PerUnit: f(2), KB: f(1)},
			{Kind: "write", PerUnit: f(0.1), KB: f(2)},
		}}},
	}
	res, err := Scout(spec)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := readUnits(t, res, "table"), 1e6*(7+2); math.Abs(got-want) > 1e-6 {
		t.Errorf("reads: %v units, want %v", got, want)
	}
	for _, n := range res.Nodes {
		for _, c := range n.Costs {
			if n.ID == "table" && c.Name == "Write request units" && math.Abs(c.Quantity-1e6*0.1*2) > 1e-6 {
				t.Errorf("writes: %v units, want %v", c.Quantity, 1e6*0.1*2)
			}
		}
	}
	// An operation of no size needs the table's item size.
	spec.Edges[0].Ops[0].KB = nil
	res, _ = Scout(spec)
	for _, n := range res.Nodes {
		if n.ID == "table" && !strings.Contains(n.Error, "size of some operations is unknown") {
			t.Errorf("an unsized read with no item size is an error: %q", n.Error)
		}
	}
}
