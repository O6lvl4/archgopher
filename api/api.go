// Package api is the JSON-in, JSON-out surface of arch-scouter. The browser
// build calls it through WebAssembly; it holds no logic of its own, so the
// web UI and the CLI read an architecture the same way.
package api

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"testing/fstest"

	"github.com/O6lvl4/arch-scouter/aws"
	"github.com/O6lvl4/arch-scouter/scout"
	"github.com/O6lvl4/arch-scouter/terraform"
)

// CatalogEntry is a scouter with the fields a form needs.
type CatalogEntry struct {
	scout.Meta
	Attributes  []scout.Field `json:"attributes"`
	Assumptions []scout.Field `json:"assumptions"`
}

// Catalog lists every scouter, sorted by type.
func Catalog() []CatalogEntry {
	reg := aws.Registry()
	var out []CatalogEntry
	for _, t := range reg.Types() {
		s := reg[t]
		out = append(out, CatalogEntry{Meta: s.Meta(), Attributes: s.Attributes(), Assumptions: s.Assumptions()})
	}
	return out
}

// Scout reads a declaration.
func Scout(spec scout.Spec) (scout.Result, error) {
	books, err := aws.Books()
	if err != nil {
		return scout.Result{}, err
	}
	return scout.Run(spec, aws.Registry(), books)
}

// TerraformRequest carries a Terraform tree as file contents keyed by
// slash-separated path, and the directory of the root module inside it.
type TerraformRequest struct {
	Files map[string]string `json:"files"`
	Root  string            `json:"root"`
	Vars  map[string]string `json:"vars,omitempty"`
	Name  string            `json:"name,omitempty"`
	// Merge folds the result into this declaration when set.
	Merge *scout.Spec `json:"merge,omitempty"`
}

// TerraformResponse is the declaration built from Terraform.
type TerraformResponse struct {
	Spec     scout.Spec `json:"spec"`
	Warnings []string   `json:"warnings"`
}

// Terraform builds a declaration from files held in memory.
func Terraform(req TerraformRequest) (TerraformResponse, error) {
	fsys := fstest.MapFS{}
	for name, data := range req.Files {
		fsys[strings.TrimPrefix(path.Clean(name), "/")] = &fstest.MapFile{Data: []byte(data)}
	}
	root := path.Clean(strings.TrimPrefix(req.Root, "/"))
	ev, err := terraform.EvaluateFS(fsys, root, terraform.Options{Vars: req.Vars})
	if err != nil {
		return TerraformResponse{}, err
	}
	name := req.Name
	if name == "" {
		name = path.Base(root)
	}
	spec, warnings := terraform.Build(ev, aws.TerraformRules(), name)
	if req.Merge != nil {
		var w []string
		spec, w = terraform.Merge(*req.Merge, spec)
		warnings = append(warnings, w...)
	}
	if spec.Region == "" {
		spec.Region = "us-east-1"
		warnings = append(warnings, "no region found in the aws provider; using us-east-1")
	}
	if warnings == nil {
		warnings = []string{}
	}
	return TerraformResponse{Spec: spec, Warnings: warnings}, nil
}

// RootCandidates lists the directories that hold .tf files, the choices for a root module.
func RootCandidates(files map[string]string) []string {
	set := map[string]bool{}
	for name := range files {
		if strings.HasSuffix(name, ".tf") && !strings.Contains(name, "/.terraform/") {
			set[path.Dir(strings.TrimPrefix(name, "/"))] = true
		}
	}
	out := make([]string, 0, len(set))
	for d := range set {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

// Regions lists the regions the price book covers.
func Regions() ([]string, error) {
	books, err := aws.Books()
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, e := range books.Prices {
		for r := range e.Values {
			if r != scout.AnyRegion {
				set[r] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for r := range set {
		out = append(out, r)
	}
	sort.Strings(out)
	return out, nil
}

// ParseYAML reads a declaration from YAML.
func ParseYAML(text string) (scout.Spec, error) { return scout.ParseSpec([]byte(text)) }

// MarshalYAML writes a declaration as YAML.
func MarshalYAML(spec scout.Spec) (string, error) {
	b, err := scout.MarshalSpec(spec)
	return string(b), err
}

// Call dispatches one JSON request by name and always returns a JSON envelope
// {"ok": value} or {"error": message}. It is what the WebAssembly entry exposes.
func Call(name, input string) string {
	out, err := call(name, input)
	if err != nil {
		b, _ := json.Marshal(map[string]string{"error": err.Error()})
		return string(b)
	}
	b, err := json.Marshal(map[string]any{"ok": out})
	if err != nil {
		b, _ = json.Marshal(map[string]string{"error": err.Error()})
	}
	return string(b)
}

func call(name, input string) (any, error) {
	switch name {
	case "catalog":
		return Catalog(), nil
	case "regions":
		return Regions()
	case "scout":
		var spec scout.Spec
		if err := json.Unmarshal([]byte(input), &spec); err != nil {
			return nil, err
		}
		return Scout(spec)
	case "terraform":
		var req TerraformRequest
		if err := json.Unmarshal([]byte(input), &req); err != nil {
			return nil, err
		}
		return Terraform(req)
	case "roots":
		var files map[string]string
		if err := json.Unmarshal([]byte(input), &files); err != nil {
			return nil, err
		}
		return RootCandidates(files), nil
	case "parseYaml":
		return ParseYAML(input)
	case "toYaml":
		var spec scout.Spec
		if err := json.Unmarshal([]byte(input), &spec); err != nil {
			return nil, err
		}
		return MarshalYAML(spec)
	}
	return nil, fmt.Errorf("unknown call %q", name)
}
