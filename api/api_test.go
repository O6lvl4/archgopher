package api

import (
	"encoding/json"
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
