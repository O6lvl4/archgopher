// Package book holds reference books: prices, quotas and SLAs keyed by id,
// with a value per region, a unit, a source and whether someone verified it.
package book

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
)

// AnyRegion keys a value that holds in every region (SLAs, most quotas).
const AnyRegion = "*"

// Name names one of the three reference books.
type Name string

const (
	Prices Name = "prices"
	Quotas Name = "quotas"
	SLAs   Name = "slas"
)

// Entry is one row of a reference book, with a value per region.
type Entry struct {
	// Unit is what one counted item is ("request", "GB-month"). Readings must use the same unit.
	Unit string `json:"unit"`
	// Per is how many units the value covers (1e6 for "per million requests"); 0 means 1.
	Per    float64 `json:"per,omitempty"`
	Source string  `json:"source"`
	Note   string  `json:"note,omitempty"`
	// Sync tells the sync command how to fetch the value. The engine ignores it.
	// In a table, "{row}" in it stands for the row key.
	Sync   json.RawMessage  `json:"sync,omitempty"`
	Values map[string]Value `json:"values,omitempty"`
	// Rows make the entry a table: one row per key (an instance type), one
	// number per region, null where the row is not offered. Row "k" of
	// "aws.ec2.linux" is looked up as "aws.ec2.linux.k". A table keeps
	// thousands of prices that differ only by one attribute compact, with
	// one sync spec for all of them.
	Rows map[string]map[string]*float64 `json:"rows,omitempty"`
	// Verified and CheckedAt hold for every value of a table.
	Verified  bool   `json:"verified,omitempty"`
	CheckedAt string `json:"checkedAt,omitempty"`
}

// Value is the number for one region. A nil Value means "not known".
type Value struct {
	Value *float64 `json:"value"`
	// Verified is true when a person checked the source or the value was synced from the provider.
	Verified  bool   `json:"verified"`
	CheckedAt string `json:"checkedAt,omitempty"`
}

// Book is a reference book keyed by ID ("aws.lambda.requests").
type Book map[string]Entry

// Books bundles the three reference books.
type Books struct {
	Prices Book `json:"prices"`
	Quotas Book `json:"quotas"`
	SLAs   Book `json:"slas"`
}

// Book returns the book by name.
func (b Books) Book(name Name) Book {
	switch name {
	case Prices:
		return b.Prices
	case Quotas:
		return b.Quotas
	default:
		return b.SLAs
	}
}

// Lookup finds the value for a region, falling back to AnyRegion.
func (b Book) Lookup(id, region string) (Entry, Value, error) {
	e, ok := b[id]
	if !ok {
		return Entry{}, Value{}, fmt.Errorf("no reference entry %q", id)
	}
	if v, ok := e.Values[region]; ok {
		return e, v, nil
	}
	if v, ok := e.Values[AnyRegion]; ok {
		return e, v, nil
	}
	return e, Value{}, fmt.Errorf("reference entry %q has no value for region %s (has %v)", id, region, e.regions())
}

// PerUnit converts the stored value to the value of a single unit.
func (e Entry) PerUnit(v float64) float64 {
	if e.Per == 0 {
		return v
	}
	return v / e.Per
}

func (e Entry) regions() []string {
	out := make([]string, 0, len(e.Values))
	for r := range e.Values {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// Marshal writes a book in its canonical form: sorted keys, two-space
// indent, no HTML escaping. Every writer of the bundled books uses it, so a
// sync that changes nothing leaves the file byte for byte the same.
func Marshal(b Book) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(b); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Load reads prices.json, quotas.json and slas.json from dir in fsys. A
// missing file is an empty book: a service may have prices but no quotas.
func Load(fsys fs.FS, dir string) (Books, error) {
	b := Books{Prices: Book{}, Quotas: Book{}, SLAs: Book{}}
	for _, name := range []Name{Prices, Quotas, SLAs} {
		data, err := fs.ReadFile(fsys, path.Join(dir, string(name)+".json"))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return b, err
		}
		var book Book
		if err := json.Unmarshal(data, &book); err != nil {
			return b, fmt.Errorf("%s/%s.json: %w", dir, name, err)
		}
		flat, err := book.Flatten()
		if err != nil {
			return b, fmt.Errorf("%s/%s.json: %w", dir, name, err)
		}
		*b.ref(name) = flat
	}
	return b, nil
}

// Flatten turns every table into one entry per row, keyed "<id>.<row>".
func (b Book) Flatten() (Book, error) {
	out := Book{}
	for id, e := range b {
		if len(e.Rows) == 0 {
			out[id] = e
			continue
		}
		if len(e.Values) > 0 {
			return nil, fmt.Errorf("%q has both values and rows", id)
		}
		for key, row := range e.Rows {
			full := id + "." + key
			if _, dup := b[full]; dup {
				return nil, fmt.Errorf("%q is both an entry and a row of %q", full, id)
			}
			values := map[string]Value{}
			for region, v := range row {
				values[region] = Value{Value: v, Verified: e.Verified, CheckedAt: e.CheckedAt}
			}
			out[full] = Entry{Unit: e.Unit, Per: e.Per, Source: e.Source, Note: e.Note, Values: values}
		}
	}
	return out, nil
}

func (b *Books) ref(name Name) *Book {
	switch name {
	case Prices:
		return &b.Prices
	case Quotas:
		return &b.Quotas
	default:
		return &b.SLAs
	}
}

// Merge combines books, refusing an id defined twice.
func Merge(parts ...Books) (Books, error) {
	out := Books{Prices: Book{}, Quotas: Book{}, SLAs: Book{}}
	for _, p := range parts {
		for _, name := range []Name{Prices, Quotas, SLAs} {
			dst := *out.ref(name)
			for id, e := range *p.ref(name) {
				if _, dup := dst[id]; dup {
					return out, fmt.Errorf("%s %q is defined twice", name, id)
				}
				dst[id] = e
			}
		}
	}
	return out, nil
}
