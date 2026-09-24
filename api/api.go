// Package api is the JSON-in, JSON-out surface of archgopher. The browser
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

	"github.com/O6lvl4/archgopher/cloud"
	"github.com/O6lvl4/archgopher/engine"
	"github.com/O6lvl4/archgopher/field"
	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/pattern"
	"github.com/O6lvl4/archgopher/scouter"
	"github.com/O6lvl4/archgopher/terraform/eval"
	"github.com/O6lvl4/archgopher/terraform/infer"
	"github.com/O6lvl4/archgopher/terraform/merge"
)

// CatalogEntry is a scouter with the fields a form needs.
type CatalogEntry struct {
	scouter.Meta
	Attributes  []field.Field `json:"attributes"`
	Assumptions []field.Field `json:"assumptions"`
}

// Catalog lists every scouter, then every pattern, each sorted by type.
// A pattern's parameters are its assumptions: it is placed and edited like a node.
func Catalog() []CatalogEntry {
	reg := cloud.Registry()
	var out []CatalogEntry
	for _, t := range reg.Types() {
		s := reg[t]
		out = append(out, CatalogEntry{Meta: s.Meta(), Attributes: s.Attributes(), Assumptions: s.Assumptions()})
	}
	patterns := cloud.Patterns()
	for _, t := range patterns.Types() {
		p := patterns[t]
		out = append(out, CatalogEntry{Meta: p.Meta(), Attributes: []field.Field{}, Assumptions: p.Params()})
	}
	return out
}

// Scout reads a declaration: patterns expand, the engine runs, and each
// pattern gets a rolled-up result.
func Scout(spec model.Spec) (engine.Result, error) {
	books, err := cloud.Books()
	if err != nil {
		return engine.Result{}, err
	}
	patterns := cloud.Patterns()
	expanded, exp, err := pattern.Expand(spec, patterns)
	if err != nil {
		return engine.Result{}, err
	}
	res, err := engine.Run(expanded, cloud.Registry(), books)
	if err != nil {
		return engine.Result{}, err
	}
	return pattern.Rollup(res, spec, exp, patterns), nil
}

// TerraformRequest carries a Terraform tree as file contents keyed by
// slash-separated path, and the directory of the root module inside it.
type TerraformRequest struct {
	Files map[string]string `json:"files"`
	Root  string            `json:"root"`
	Vars  map[string]string `json:"vars,omitempty"`
	Name  string            `json:"name,omitempty"`
	// Merge folds the result into this declaration when set.
	Merge *model.Spec `json:"merge,omitempty"`
}

// TerraformResponse is the declaration built from Terraform.
type TerraformResponse struct {
	Spec     model.Spec `json:"spec"`
	Warnings []string   `json:"warnings"`
}

// Terraform builds a declaration from files held in memory.
func Terraform(req TerraformRequest) (TerraformResponse, error) {
	fsys := fstest.MapFS{}
	for name, data := range req.Files {
		fsys[strings.TrimPrefix(path.Clean(name), "/")] = &fstest.MapFile{Data: []byte(data)}
	}
	root := path.Clean(strings.TrimPrefix(req.Root, "/"))
	ev, err := eval.EvaluateFS(fsys, root, eval.Options{Vars: req.Vars})
	if err != nil {
		return TerraformResponse{}, err
	}
	name := req.Name
	if name == "" {
		name = path.Base(root)
	}
	spec, warnings := infer.Build(ev, cloud.TerraformRules(), name)
	if req.Merge != nil {
		var w []string
		spec, w = merge.Merge(*req.Merge, spec)
		warnings = append(warnings, w...)
	}
	if spec.Region == "" {
		spec.Region = "us-east-1"
		warnings = append(warnings, "no region found in an aws provider block or an azurerm location; using us-east-1")
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

// Regions lists each provider's regions.
func Regions() ([]cloud.RegionGroup, error) {
	return cloud.Regions()
}

// ParseYAML reads a declaration from YAML.
func ParseYAML(text string) (model.Spec, error) { return model.ParseSpec([]byte(text)) }

// MarshalYAML writes a declaration as YAML.
func MarshalYAML(spec model.Spec) (string, error) {
	b, err := model.MarshalSpec(spec)
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
		var spec model.Spec
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
		var spec model.Spec
		if err := json.Unmarshal([]byte(input), &spec); err != nil {
			return nil, err
		}
		return MarshalYAML(spec)
	}
	return nil, fmt.Errorf("unknown call %q", name)
}
