package api

import (
	"math"

	"encoding/json"
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
			{ID: "table", Type: "aws_dynamodb_table", Assumptions: map[string]any{"itemSizeKb": 1, "storageGb": 1}},
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
	if len(res.Warnings) == 0 || !strings.Contains(strings.Join(res.Warnings, " "), "fn -> table") {
		t.Errorf("kb on an edge leaving the group is reported: %v", res.Warnings)
	}
}
