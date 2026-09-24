// Package cloudflare is the Cloudflare provider. Resources live as data in
// catalog/cloudflare, one directory per resource type; this package loads them and
// adds what is not per resource.
package cloudflare

import (
	"sort"
	"sync"

	"github.com/O6lvl4/archgopher/book"
	"github.com/O6lvl4/archgopher/catalog"
	"github.com/O6lvl4/archgopher/definition"
	"github.com/O6lvl4/archgopher/scouter"
	"github.com/O6lvl4/archgopher/terraform/infer"
)

var (
	once   sync.Once
	units  []definition.Unit
	errCat error
)

// Units loads the catalog once.
func Units() ([]definition.Unit, error) {
	once.Do(func() { units, errCat = definition.LoadAll(catalog.Cloudflare, catalog.CloudflareRoot) })
	return units, errCat
}

func mustUnits() []definition.Unit {
	u, err := Units()
	if err != nil {
		// The catalog is embedded and tested; a broken one is a build defect.
		panic("archgopher: Cloudflare catalog: " + err.Error())
	}
	return u
}

// Registry returns every Cloudflare resource plus the provider-neutral entry.
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

// Regions lists the regions the price books name; Cloudflare prices are all
// "*", so it is empty.
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

// TerraformRules combine every resource's rules. Cloudflare has no regions:
// every price is the same everywhere, so it reads no region and its resources
// price in whatever region the declaration names.
func TerraformRules() infer.Rules {
	parts := []infer.Rules{{Scouters: Registry()}}
	for _, u := range mustUnits() {
		if u.Resource != nil {
			parts = append(parts, u.Resource.Rules())
		}
	}
	return infer.Combine(parts...)
}
