// Package gcp is the Google Cloud provider. Resources live as data in
// catalog/gcp, one directory per resource type; this package loads them and
// adds what is not per resource: the region reader and account-wide rules.
package gcp

import (
	"strings"

	"github.com/O6lvl4/archgopher/book"
	"github.com/O6lvl4/archgopher/catalog"
	"github.com/O6lvl4/archgopher/definition"
	"github.com/O6lvl4/archgopher/provider/internal/embedded"
	"github.com/O6lvl4/archgopher/scouter"
	"github.com/O6lvl4/archgopher/terraform/eval"
	"github.com/O6lvl4/archgopher/terraform/infer"
)

var cat = embedded.New("Google Cloud", catalog.GCP, catalog.GCPRoot)

// Units loads the catalog once.
func Units() ([]definition.Unit, error) { return cat.Units() }

// Registry returns every Google Cloud resource plus the provider-neutral entry.
func Registry() scouter.Registry { return cat.Registry() }

// Books merges the books of every unit; an id owned by two units is an error.
func Books() (book.Books, error) { return cat.Books() }

// Regions lists every region the price books cover, sorted.
func Regions() ([]string, error) { return cat.Regions() }

// TerraformRules combine the region reader and every resource's rules.
func TerraformRules() infer.Rules {
	return cat.Rules(infer.Rules{
		Scouters: Registry(), Region: Region,
		Free:       cat.FreeTypes(),
		IgnoreRefs: []string{"service_account", "service_account_email", "encryption_key_name", "kms_key_name"},
	})
}

// Region reads the google provider's region, or else the most common region
// or location of the google resources, lowercased (buckets say ASIA-NORTHEAST1).
func Region(ev *eval.Evaluated) string {
	if r, ok := ev.Providers["google"]["region"].(string); ok && r != "" {
		return r
	}
	count := map[string]int{}
	for _, r := range ev.Resources {
		if loc := location(r); loc != "" {
			count[loc]++
		}
	}
	best, n := "", 0
	for loc, c := range count {
		if c > n || (c == n && loc < best) {
			best, n = loc, c
		}
	}
	return best
}

// location is where a google resource is placed, lowercased: its region, or
// else its location. A location without a dash is a multi-region ("US"), not
// a region, and is skipped.
func location(r *eval.Resource) string {
	if !strings.HasPrefix(r.Type, "google_") {
		return ""
	}
	for _, k := range []string{"region", "location"} {
		if loc, ok := r.Attrs[k].(string); ok && strings.Contains(loc, "-") {
			return strings.ToLower(loc)
		}
	}
	return ""
}
