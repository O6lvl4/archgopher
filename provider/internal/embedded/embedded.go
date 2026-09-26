// Package embedded loads a provider's catalog, which is embedded in the
// binary, and derives from it what every provider offers: the scouters, the
// merged books, the regions they price and the Terraform rules.
package embedded

import (
	"io/fs"
	"maps"
	"slices"
	"sync"

	"github.com/O6lvl4/archgopher/book"
	"github.com/O6lvl4/archgopher/definition"
	"github.com/O6lvl4/archgopher/scouter"
	"github.com/O6lvl4/archgopher/terraform/infer"
)

// Catalog is one provider's catalog, loaded on first use.
type Catalog struct {
	name string
	fsys fs.FS
	root string

	once  sync.Once
	units []definition.Unit
	err   error
}

// New names the catalog under root in fsys; name is the provider's, for
// messages ("AWS").
func New(name string, fsys fs.FS, root string) *Catalog {
	return &Catalog{name: name, fsys: fsys, root: root}
}

// Units loads the catalog once.
func (c *Catalog) Units() ([]definition.Unit, error) {
	c.once.Do(func() { c.units, c.err = definition.LoadAll(c.fsys, c.root) })
	return c.units, c.err
}

// MustUnits is Units for callers that have no error to return.
func (c *Catalog) MustUnits() []definition.Unit {
	units, err := c.Units()
	return must(c.name+" catalog", units, err)
}

// FreeTypes are the resource types that cost nothing by themselves.
func (c *Catalog) FreeTypes() map[string]bool {
	free, err := definition.FreeTypes(c.fsys, c.root)
	return must(c.name+" free types", free, err)
}

// must stops on a catalog that does not load. The catalog is embedded and
// tested, so a broken one is a build defect, not an input error.
func must[T any](what string, v T, err error) T {
	if err != nil {
		panic("archgopher: " + what + ": " + err.Error())
	}
	return v
}

// Registry returns every resource of the catalog plus the provider-neutral entry.
func (c *Catalog) Registry() scouter.Registry {
	reg := scouter.Registry{}
	reg.Register(scouter.EntryScouter)
	for _, u := range c.MustUnits() {
		if u.Resource != nil {
			reg.Register(u.Resource)
		}
	}
	return reg
}

// Books merges the books of every unit, with every price in US dollars; an
// id owned by two units is an error.
func (c *Catalog) Books() (book.Books, error) {
	us, err := c.Units()
	if err != nil {
		return book.Books{}, err
	}
	parts := make([]book.Books, 0, len(us))
	for _, u := range us {
		parts = append(parts, u.Books)
	}
	books, err := book.Merge(parts...)
	if err != nil {
		return book.Books{}, err
	}
	books.Prices, err = books.Prices.InUSD()
	return books, err
}

// Regions lists every region the price books name, sorted; "*" is not one.
func (c *Catalog) Regions() ([]string, error) {
	books, err := c.Books()
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, e := range books.Prices {
		for r := range e.Values {
			set[r] = true
		}
	}
	delete(set, book.AnyRegion)
	// Empty, not nil, for a provider whose prices are all "*".
	out := slices.AppendSeq(make([]string, 0, len(set)), maps.Keys(set))
	slices.Sort(out)
	return out, nil
}

// Rules combines the provider's own rules, in order, with every resource's.
func (c *Catalog) Rules(own ...infer.Rules) infer.Rules {
	parts := slices.Clone(own)
	for _, u := range c.MustUnits() {
		if u.Resource != nil {
			parts = append(parts, u.Resource.Rules())
		}
	}
	return infer.Combine(parts...)
}
