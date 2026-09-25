// Package azure is the Azure provider. Resources live as data in
// catalog/azure, one directory per resource type; this package loads them and
// adds what is not per resource: the region reader, the role assignment edge
// source and account-wide rules.
package azure

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
	once.Do(func() { units, errCat = definition.LoadAll(catalog.Azure, catalog.AzureRoot) })
	return units, errCat
}

func mustUnits() []definition.Unit {
	u, err := Units()
	if err != nil {
		// The catalog is embedded and tested; a broken one is a build defect.
		panic("archgopher: Azure catalog: " + err.Error())
	}
	return u
}

// Registry returns every Azure resource plus the provider-neutral entry.
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

// TerraformRules combine the region reader, the role assignment edge source
// and every resource's rules. Identities and role assignments name what they
// grant; they are not calls.
func TerraformRules() infer.Rules {
	parts := []infer.Rules{{
		Scouters:   Registry(),
		Free:       freeTypes(),
		Region:     Region,
		Sources:    []infer.EdgeSource{RBAC(roles())},
		IgnoreRefs: []string{"identity", "key_vault_reference_identity_id"},
	}}
	for _, u := range mustUnits() {
		if u.Resource != nil {
			parts = append(parts, u.Resource.Rules())
		}
	}
	return infer.Combine(parts...)
}

// Region reads the region the azurerm resources are placed in: the most
// common location, normalized to the ARM name ("Japan East" is japaneast).
func Region(ev *eval.Evaluated) string {
	count := map[string]int{}
	for _, r := range ev.Resources {
		if !strings.HasPrefix(r.Type, "azurerm_") {
			continue
		}
		if loc, ok := r.Attrs["location"].(string); ok && loc != "" {
			count[armName(loc)]++
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

// armName turns a display location into its ARM name.
func armName(loc string) string {
	return strings.ToLower(strings.ReplaceAll(loc, " ", ""))
}

// freeTypes are the resource types that cost nothing by themselves.
func freeTypes() map[string]bool {
	f, err := definition.FreeTypes(catalog.Azure, catalog.AzureRoot)
	if err != nil {
		panic("archgopher: Azure free types: " + err.Error())
	}
	return f
}
