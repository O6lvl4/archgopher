// Package conoha is the ConoHa VPS provider (GMO Internet's conohavps
// Terraform provider). Resources live as data in catalog/conoha, one
// directory per resource type; this package loads them and adds what is not
// per resource.
package conoha

import (
	"github.com/O6lvl4/archgopher/book"
	"github.com/O6lvl4/archgopher/catalog"
	"github.com/O6lvl4/archgopher/definition"
	"github.com/O6lvl4/archgopher/provider/internal/embedded"
	"github.com/O6lvl4/archgopher/scouter"
	"github.com/O6lvl4/archgopher/terraform/infer"
)

var cat = embedded.New("ConoHa", catalog.ConoHa, catalog.ConoHaRoot)

// Units loads the catalog once.
func Units() ([]definition.Unit, error) { return cat.Units() }

// Registry returns every ConoHa resource plus the provider-neutral entry.
func Registry() scouter.Registry { return cat.Registry() }

// Books merges the books of every unit, with the yen prices in US dollars.
func Books() (book.Books, error) { return cat.Books() }

// Regions lists the regions the price books name. ConoHa VPS runs in one
// region, Tokyo (c3j1), and its prices are all "*", so it is empty.
func Regions() ([]string, error) { return cat.Regions() }

// TerraformRules combine every resource's rules. With one region and its
// prices under "*", it reads no region: its resources price in whatever
// region the declaration names.
func TerraformRules() infer.Rules {
	return cat.Rules(infer.Rules{Scouters: Registry(), Free: cat.FreeTypes()})
}
