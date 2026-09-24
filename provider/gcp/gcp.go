// Package gcp is the Google Cloud provider. Resources live as data in
// catalog/gcp, one directory per resource type; this package loads them and
// adds what is not per resource: the region reader and account-wide rules.
package gcp

import (
	"sort"
	"strings"
	"sync"

	"github.com/O6lvl4/archgopher/book"
	"github.com/O6lvl4/archgopher/catalog"
	"github.com/O6lvl4/archgopher/definition"
	"github.com/O6lvl4/archgopher/scouter"
	"github.com/O6lvl4/archgopher/terraform/eval"
	"github.com/O6lvl4/archgopher/terraform/infer"
)

var (
	once   sync.Once
	units  []definition.Unit
	errCat error
)

// Units loads the catalog once.
func Units() ([]definition.Unit, error) {
	once.Do(func() { units, errCat = definition.LoadAll(catalog.GCP, catalog.GCPRoot) })
	return units, errCat
}

func mustUnits() []definition.Unit {
	u, err := Units()
	if err != nil {
		// The catalog is embedded and tested; a broken one is a build defect.
		panic("archgopher: Google Cloud catalog: " + err.Error())
	}
	return u
}

// Registry returns every Google Cloud resource plus the provider-neutral entry.
func Registry() scouter.Registry {
	reg := scouter.Registry{}
	reg.Register(scouter.EntryScouter)
	for _, u := range mustUnits() {
		if u.Resource != nil {
			reg.Register(u.Resource)
		}
	}
	return reg
}

// Books merges the books of every unit; an id owned by two units is an error.
func Books() (book.Books, error) {
	us, err := Units()
	if err != nil {
		return book.Books{}, err
	}
	parts := make([]book.Books, 0, len(us))
	for _, u := range us {
		parts = append(parts, u.Books)
	}
	return book.Merge(parts...)
}

// Regions lists every region the price books cover, sorted.
func Regions() ([]string, error) {
	books, err := Books()
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, e := range books.Prices {
		for r := range e.Values {
			if r != book.AnyRegion {
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

// TerraformRules combine the region reader and every resource's rules.
func TerraformRules() infer.Rules {
	parts := []infer.Rules{{Scouters: Registry(), Region: Region, IgnoreRefs: []string{"service_account", "service_account_email", "encryption_key_name", "kms_key_name"}}}
	for _, u := range mustUnits() {
		if u.Resource != nil {
			parts = append(parts, u.Resource.Rules())
		}
	}
	return infer.Combine(parts...)
}

// Region reads the google provider's region, or else the most common region
// or location of the google resources, lowercased (buckets say ASIA-NORTHEAST1).
func Region(ev *eval.Evaluated) string {
	if r, ok := ev.Providers["google"]["region"].(string); ok && r != "" {
		return r
	}
	count := map[string]int{}
	for _, r := range ev.Resources {
		if !strings.HasPrefix(r.Type, "google_") {
			continue
		}
		for _, k := range []string{"region", "location"} {
			if loc, ok := r.Attrs[k].(string); ok && strings.Contains(loc, "-") {
				count[strings.ToLower(loc)]++
				break
			}
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
