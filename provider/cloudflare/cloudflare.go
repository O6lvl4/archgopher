// Package cloudflare is the Cloudflare provider. Resources live as data in
// catalog/cloudflare, one directory per resource type; this package loads them and
// adds what is not per resource.
package cloudflare

import (
	"github.com/O6lvl4/archgopher/book"
	"github.com/O6lvl4/archgopher/catalog"
	"github.com/O6lvl4/archgopher/definition"
	"github.com/O6lvl4/archgopher/provider/internal/embedded"
	"github.com/O6lvl4/archgopher/scouter"
	"github.com/O6lvl4/archgopher/terraform/infer"
)

var cat = embedded.New("Cloudflare", catalog.Cloudflare, catalog.CloudflareRoot)

// Units loads the catalog once.
func Units() ([]definition.Unit, error) { return cat.Units() }

// Registry returns every Cloudflare resource plus the provider-neutral entry.
func Registry() scouter.Registry { return cat.Registry() }

// Books merges the books of every unit; an id owned by two units is an error.
func Books() (book.Books, error) { return cat.Books() }

// Regions lists the regions the price books name; Cloudflare prices are all
// "*", so it is empty.
func Regions() ([]string, error) { return cat.Regions() }

// TerraformRules combine every resource's rules. Cloudflare has no regions:
// every price is the same everywhere, so it reads no region and its resources
// price in whatever region the declaration names.
func TerraformRules() infer.Rules {
	return cat.Rules(infer.Rules{Scouters: Registry()})
}
