// Package azure is the Azure provider. Resources live as data in
// catalog/azure, one directory per resource type; this package loads them and
// adds what is not per resource: the region reader, the role assignment edge
// source and account-wide rules.
package azure

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

var cat = embedded.New("Azure", catalog.Azure, catalog.AzureRoot)

// Units loads the catalog once.
func Units() ([]definition.Unit, error) { return cat.Units() }

// Registry returns every Azure resource plus the provider-neutral entry.
func Registry() scouter.Registry { return cat.Registry() }

// Books merges the books of every unit; an id owned by two units is an error.
func Books() (book.Books, error) { return cat.Books() }

// Regions lists every region the price books cover, sorted.
func Regions() ([]string, error) { return cat.Regions() }

// TerraformRules combine the region reader, the role assignment edge source
// and every resource's rules. Identities and role assignments name what they
// grant; they are not calls.
func TerraformRules() infer.Rules {
	return cat.Rules(infer.Rules{
		Scouters:   Registry(),
		Free:       cat.FreeTypes(),
		Region:     Region,
		Sources:    []infer.EdgeSource{RBAC(roles())},
		IgnoreRefs: []string{"identity", "key_vault_reference_identity_id"},
	})
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
