// Package catalogtest holds the checks every provider catalog must pass: the
// books are complete for every region, well formed and canonical, every
// resource reads them, and every worked example still holds. A provider's
// test calls these with its own catalog.
package catalogtest

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/O6lvl4/archgopher/book"
	"github.com/O6lvl4/archgopher/definition"
	"github.com/O6lvl4/archgopher/field"
	"github.com/O6lvl4/archgopher/meter"
	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/scouter"
)

// Catalog is one provider's catalog under test.
type Catalog struct {
	// Dir is the catalog directory relative to the test, "../../catalog/aws".
	Dir      string
	Units    []definition.Unit
	Books    book.Books
	Registry scouter.Registry
	Regions  []string
	// Attrs and Assume make a resource take its main code path.
	Attrs, Assume map[string]map[string]any
	// Update rewrites books and cases instead of checking them; UpdateHint is
	// the command that does it, for the failure message.
	Update     bool
	UpdateHint string
}

// EveryScouterReadsTheBooks: every resource finds every reference it reads,
// in the units it counts, in every region. Unknown values and prices the
// region does not offer are allowed; missing rows are not.
func (c Catalog) EveryScouterReadsTheBooks(t *testing.T) {
	t.Helper()
	for _, typ := range c.Registry.Types() {
		s := c.Registry[typ]
		for _, region := range c.Regions {
			n := model.Node{ID: "n", Type: typ, Attributes: c.Attrs[typ], Assumptions: sample(s.Assumptions(), c.Assume[typ])}
			if typ == scouter.EntryType {
				n.Load = &model.Load{Monthly: 1, PeakPerSecond: 1}
			}
			d := model.Demand{}
			for _, k := range s.Meta().Kinds {
				d[k] = model.Load{Monthly: 1e6, PeakPerSecond: 10}
			}
			if err := s.Scout(n, d, meter.NewRecorder(region, c.Books)); err != nil && !onlyNotOffered(err) {
				t.Errorf("%s in %s: %v", typ, region, err)
			}
		}
	}
}

// EveryRegionIsComplete keeps a region all or nothing: every row that varies
// by region has a value, or a verified "not offered", for every region.
func (c Catalog) EveryRegionIsComplete(t *testing.T) {
	t.Helper()
	for _, name := range []book.Name{book.Prices, book.Quotas, book.SLAs} {
		for id, e := range c.Books.Book(name) {
			if _, any := e.Values[book.AnyRegion]; any {
				continue
			}
			for _, r := range c.Regions {
				if _, ok := e.Values[r]; !ok {
					t.Errorf("%s %q has no value for %s", name, id, r)
				}
			}
		}
	}
}

// BooksAreWellFormed: every row has a unit, an https source and values.
func (c Catalog) BooksAreWellFormed(t *testing.T) {
	t.Helper()
	for _, name := range []book.Name{book.Prices, book.Quotas, book.SLAs} {
		for id, e := range c.Books.Book(name) {
			if e.Unit == "" || !strings.HasPrefix(e.Source, "https://") || len(e.Values) == 0 {
				t.Errorf("%s %s: needs a unit, an https source and values", name, id)
			}
		}
	}
}

// BooksAreCanonical keeps the book files in the one form sync writes, so a
// sync with unchanged values produces no diff.
func (c Catalog) BooksAreCanonical(t *testing.T) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(c.Dir, "*", "books", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no book files under %s: %v", c.Dir, err)
	}
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var b book.Book
		if err := json.Unmarshal(raw, &b); err != nil {
			t.Fatal(err)
		}
		want, err := book.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(raw, want) {
			continue
		}
		if c.Update {
			if err := os.WriteFile(path, want, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		t.Errorf("%s is not canonical; run: %s", path, c.UpdateHint)
	}
}

// Cases checks every resource's worked examples, and requires them.
func (c Catalog) Cases(t *testing.T) {
	t.Helper()
	for _, u := range c.Units {
		if u.Resource == nil {
			continue
		}
		if len(u.Cases) == 0 {
			t.Errorf("%s has no cases.yaml", u.Name)
			continue
		}
		for i, cs := range u.Cases {
			if strings.Contains(cs.Error, "no reference entry") || strings.Contains(cs.Error, "but the reading counts") {
				t.Errorf("%s case %q reads a missing or mismatched reference: %s", u.Name, cs.Name, cs.Error)
			}
			if c.Update {
				costs, limits, err := cs.Read(u.Resource, c.Books)
				u.Cases[i].Costs, u.Cases[i].Limits, u.Cases[i].Error = costs, limits, ""
				if err != nil {
					u.Cases[i].Error = err.Error()
				}
				continue
			}
			if err := cs.Check(u.Resource, c.Books); err != nil {
				t.Errorf("%s: %v", u.Name, err)
			}
		}
		if c.Update {
			c.writeCases(t, u)
		}
	}
}

func (c Catalog) writeCases(t *testing.T, u definition.Unit) {
	t.Helper()
	data, err := yaml.Marshal(u.Cases)
	if err != nil {
		t.Fatal(err)
	}
	header := "# Worked examples: given these values and this load, the resource reads this.\n# Monthly USD per cost line, peak demand per limit. After an intended change:\n#   " + c.UpdateHint + "\n"
	if err := os.WriteFile(filepath.Join(c.Dir, u.Name, "cases.yaml"), append([]byte(header), data...), 0o644); err != nil {
		t.Fatal(err)
	}
}

// sample fills required fields with a plausible value and applies overrides.
func sample(fields []field.Field, over map[string]any) map[string]any {
	out := map[string]any{}
	for _, f := range fields {
		if !f.Required {
			continue
		}
		switch f.Type {
		case field.Choice:
			out[f.Key] = f.Options[0]
		case field.Flag:
			out[f.Key] = false
		default:
			out[f.Key] = 1.0
		}
	}
	for k, v := range over {
		out[k] = v
	}
	return out
}

// onlyNotOffered reports whether every joined error is a price the region
// does not offer, which is a fact about the region, not a broken book.
func onlyNotOffered(err error) bool {
	errs := []error{err}
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		errs = j.Unwrap()
	}
	for _, e := range errs {
		var no *meter.NotOfferedError
		if !errors.As(e, &no) {
			return false
		}
	}
	return true
}
