package scout

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// AnyRegion keys a value that holds in every region (SLAs, most quotas).
const AnyRegion = "*"

// BookName names one of the three reference books.
type BookName string

const (
	Prices BookName = "prices"
	Quotas BookName = "quotas"
	SLAs   BookName = "slas"
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
	Sync   json.RawMessage  `json:"sync,omitempty"`
	Values map[string]Value `json:"values"`
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
func (b Books) Book(name BookName) Book {
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

// MarshalBook writes a book in its canonical form: sorted keys, two-space
// indent, no HTML escaping. Every writer of the bundled books uses it, so a
// sync that changes nothing leaves the file byte for byte the same.
func MarshalBook(b Book) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(b); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
