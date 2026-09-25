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
	"strconv"
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
	// Compose builds a table row out of several prices the provider does
	// list, when it lists none for the row itself: a Compute Engine machine
	// type is so many vCPU-hours, GiB-hours of memory, GPU-hours and local
	// SSD. The engine reads the rows; sync resolves each part and sums them.
	Compose map[string]Composition `json:"compose,omitempty"`
	// ComposeOf reads Compose from another table of the same book (the Spot
	// prices of the same machine types).
	ComposeOf string `json:"composeOf,omitempty"`
	// Pool says the provider bills the price on what a whole account uses,
	// not on each resource: volume tiers are counted and included units
	// given once for every reading of it. PoolAccount pools the whole
	// declaration, PoolRegion each region of it. Empty prices each reading
	// on its own.
	Pool string `json:"pool,omitempty"`
	// Included is how many units a paid plan includes each month (the
	// Workers Paid plan's requests), given once per pool. Free tiers are
	// not: they are not what the architecture costs month after month.
	Included float64 `json:"included,omitempty"`
	// Combine is CombineMax for a fee the pool pays once however many
	// readings need it (a regional fee while any dedicated instance runs):
	// the pool is billed for its largest quantity, not their sum.
	Combine string `json:"combine,omitempty"`
	// Tiered makes a table's rows volume tiers. A row key is where its tier
	// starts, counted in Unit over the pool's whole usage; the first is "0".
	// A zero-priced tier at the start is a free grant, billed at the first
	// paid tier's price.
	Tiered bool `json:"tiered,omitempty"`
	// Tiers are a tiered table's row keys as numbers, ascending. Flatten
	// fills them on the entry it keeps under the table's own id.
	Tiers []Tier `json:"-"`
}

// Pool scopes and the combine mode of Entry.
const (
	PoolAccount = "account"
	PoolRegion  = "region"
	CombineMax  = "max"
)

// Tier is one row of a tiered table: its start and the id of its row entry.
type Tier struct {
	From float64
	ID   string
}

// Billed is true when the entry is priced by pricing rules, not a plain
// quantity × value: tiers, included units or a pool.
func (e Entry) Billed() bool { return e.Pool != "" || e.Included > 0 || e.Tiered }

// Composition is how one row is made: its parts, and the regions where the
// row is offered at all (a part can be sold where the whole is not).
type Composition struct {
	In    []string `json:"in,omitempty"`
	Parts []Part   `json:"parts"`
}

// Part is one listed price and how many of its units one unit of the row
// takes: 8 vCPU-hours for one hour of an 8-vCPU machine. Name fills {part}
// in the sync filters literally; Match fills it as a regular expression, for
// a price the provider names differently from region to region. With
// overrides keys of the sync spec for this part (a license sold "global");
// WithIn does so in one region only, where the provider lists two prices
// under one name and only an id tells them apart.
type Part struct {
	Name   string                     `json:"part"`
	Match  string                     `json:"match,omitempty"`
	Times  float64                    `json:"times"`
	With   json.RawMessage            `json:"with,omitempty"`
	WithIn map[string]json.RawMessage `json:"withIn,omitempty"`
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

// Flatten turns every table into one entry per row, keyed "<id>.<row>". A
// tiered table also keeps an entry under its own id, which carries the
// pricing rules and the tiers but no values.
func (b Book) Flatten() (Book, error) {
	out := Book{}
	for id, e := range b {
		if err := e.checkRules(id); err != nil {
			return nil, err
		}
		if len(e.Rows) == 0 {
			out[id] = e
			continue
		}
		if len(e.Values) > 0 {
			return nil, fmt.Errorf("%q has both values and rows", id)
		}
		if e.Tiered {
			tiers, err := tiersOf(id, e.Rows)
			if err != nil {
				return nil, err
			}
			out[id] = Entry{Unit: e.Unit, Per: e.Per, Source: e.Source, Note: e.Note, Pool: e.Pool, Included: e.Included, Combine: e.Combine, Tiered: true, Tiers: tiers, Verified: e.Verified, CheckedAt: e.CheckedAt}
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
			row := Entry{Unit: e.Unit, Per: e.Per, Source: e.Source, Note: e.Note, Values: values}
			if !e.Tiered {
				// The rules hold for each row on its own: one pool per row.
				row.Pool, row.Included, row.Combine = e.Pool, e.Included, e.Combine
			}
			out[full] = row
		}
	}
	return out, nil
}

func (e Entry) checkRules(id string) error {
	switch {
	case e.Pool != "" && e.Pool != PoolAccount && e.Pool != PoolRegion:
		return fmt.Errorf("%q: pool is %q, not %q or %q", id, e.Pool, PoolAccount, PoolRegion)
	case e.Combine != "" && e.Combine != CombineMax:
		return fmt.Errorf("%q: combine is %q, not %q", id, e.Combine, CombineMax)
	case e.Combine != "" && e.Pool == "":
		return fmt.Errorf("%q: combine needs a pool", id)
	case e.Included < 0:
		return fmt.Errorf("%q: included must not be negative", id)
	case e.Tiered && len(e.Rows) == 0:
		return fmt.Errorf("%q: tiered needs rows", id)
	}
	return nil
}

func tiersOf(id string, rows map[string]map[string]*float64) ([]Tier, error) {
	tiers := make([]Tier, 0, len(rows))
	for key := range rows {
		from, err := strconv.ParseFloat(key, 64)
		if err != nil || from < 0 {
			return nil, fmt.Errorf("%q is tiered, so row %q must be where its tier starts", id, key)
		}
		tiers = append(tiers, Tier{From: from, ID: id + "." + key})
	}
	sort.Slice(tiers, func(i, j int) bool { return tiers[i].From < tiers[j].From })
	if tiers[0].From != 0 {
		return nil, fmt.Errorf("%q is tiered, so its first row must start at 0", id)
	}
	return tiers, nil
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
