// Package layers checks the dependency rule: every package may import only
// the packages listed for it, so arrows point one way, toward what changes
// least. A new import that breaks the rule fails here before it spreads.
package layers

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const module = "github.com/O6lvl4/archgopher/"

// allowed maps a package (or a prefix ending in /*) to the internal packages it may import.
var allowed = map[string][]string{
	// Core: the vocabulary, then L1 meters, L2 facets, scouters and the engine.
	"model": {},
	"field": {},
	"book":  {},
	"meter": {"book"},
	"facet": {"book", "meter", "model"},
	// Resources as data: a definition compiles into a scouter from facets.
	"definition": {"book", "facet", "field", "meter", "model", "scouter", "terraform/infer"},
	"catalog":    {},
	"scouter":    {"field", "meter", "model"},
	"engine":     {"book", "meter", "model", "scouter"},
	"pattern":    {"engine", "field", "meter", "model", "scouter"},
	"report":     {"engine", "meter", "model"},

	// Terraform adapter: read, evaluate, infer, merge. No provider knowledge.
	"terraform/config": {},
	"terraform/eval":   {"terraform/config"},
	"terraform/infer":  {"field", "model", "scouter", "terraform/eval"},
	"terraform/merge":  {"model"},

	// Checks every provider catalog must pass, called from provider tests.
	"internal/catalogtest": {"book", "definition", "field", "meter", "model", "scouter"},

	// AWS provider: resources are data in catalog/aws; the provider adds IAM,
	// the schedule syntax and account-wide rules.
	"provider/aws/iam":       {"terraform/eval", "terraform/infer"},
	"provider/aws/pricelist": {},
	"provider/aws/pattern":   {"model", "pattern", "scouter"},
	"provider/aws/schedule":  {"model"},
	"provider/aws":           {"book", "catalog", "definition", "scouter", "terraform/eval", "terraform/infer", "provider/aws/iam", "provider/aws/schedule"},

	// Azure provider: resources are data in catalog/azure.
	"provider/azure/retailprices": {},
	"provider/azure":              {"book", "catalog", "definition", "scouter", "terraform/eval", "terraform/infer"},

	// Google Cloud provider: prices from the Billing Catalog.
	"provider/gcp/billingcatalog": {},
	"provider/gcp":                {"book", "catalog", "definition", "scouter", "terraform/eval", "terraform/infer"},

	// The one place that lists the providers.
	"cloud": {"book", "pattern", "scouter", "terraform/infer", "provider/aws", "provider/aws/pattern", "provider/azure", "provider/gcp"},

	// Edges of the system.
	"api":            {"book", "cloud", "engine", "field", "model", "pattern", "scouter", "terraform/eval", "terraform/infer", "terraform/merge"},
	"cmd/archgopher": {"api", "book", "cloud", "model", "report", "provider/aws/pricelist", "provider/azure/retailprices", "provider/gcp/billingcatalog", "terraform/eval", "terraform/infer", "terraform/merge"},
	"cmd/wasm":       {"api"},
}

func rule(pkg string) ([]string, bool) {
	if r, ok := allowed[pkg]; ok {
		return r, true
	}
	if i := strings.LastIndex(pkg, "/"); i > 0 {
		if r, ok := allowed[pkg[:i]+"/*"]; ok {
			return r, true
		}
	}
	return nil, false
}

func permits(list []string, imp string) bool {
	for _, a := range list {
		if a == imp || (strings.HasSuffix(a, "/*") && strings.HasPrefix(imp, strings.TrimSuffix(a, "*"))) {
			return true
		}
	}
	return false
}

// imports lists the internal packages the non-test Go files of dir import.
func imports(t *testing.T, dir string) []string {
	files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	set := map[string]bool{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range parsed.Imports {
			p, _ := strconv.Unquote(spec.Path.Value)
			if strings.HasPrefix(p, module) {
				set[strings.TrimPrefix(p, module)] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func TestDependencyRule(t *testing.T) {
	root := "../.."
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		name := d.Name()
		if path != root && (strings.HasPrefix(name, ".") || name == "web" || name == "internal" || name == "testdata" || name == "examples" || name == "node_modules") {
			return filepath.SkipDir
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		imps := imports(t, path)
		if rel == "." || len(imps) == 0 && !hasGo(path) {
			return nil
		}
		list, ok := rule(rel)
		if !ok {
			t.Errorf("%s has no entry in the dependency rule; add one", rel)
			return nil
		}
		for _, imp := range imps {
			if !permits(list, imp) {
				t.Errorf("%s imports %s, which the dependency rule does not allow", rel, imp)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func hasGo(dir string) bool {
	files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	return len(files) > 0
}
