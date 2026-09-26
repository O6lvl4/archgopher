package conoha

import (
	"flag"
	"path/filepath"
	"testing"

	"github.com/O6lvl4/archgopher/internal/catalogtest"
)

var update = flag.Bool("update", false, "rewrite the bundled books in canonical form and the cases' expected values")

// attrs and assume make each resource take its main code path.
var (
	attrs = map[string]map[string]any{
		"conohavps_instance": {"flavor_id": "f2a77529-1815-43a2-bc14-1f3f6b09079c"},
		"conohavps_volume":   {"size": 200},
	}
	assume = map[string]map[string]any{}
)

// ConoHa prices are the same wherever the declaration is; read them in regions of each
// other provider to show they price wherever the declaration is.
var sampleRegions = []string{"us-east-1", "japaneast", "asia-northeast1"}

func under(t *testing.T) catalogtest.Catalog {
	t.Helper()
	books, err := Books()
	if err != nil {
		t.Fatal(err)
	}
	return catalogtest.Catalog{
		Dir: filepath.Join("..", "..", "catalog", "conoha"), Units: cat.MustUnits(), Books: books, Registry: Registry(), Regions: sampleRegions,
		Attrs: attrs, Assume: assume, Update: *update, UpdateHint: "go test ./provider/conoha -update",
	}
}

func TestEveryScouterReadsTheBooks(t *testing.T) { under(t).EveryScouterReadsTheBooks(t) }
func TestEveryRegionIsComplete(t *testing.T)     { under(t).EveryRegionIsComplete(t) }
func TestBooksAreWellFormed(t *testing.T)        { under(t).BooksAreWellFormed(t) }
func TestBooksAreCanonical(t *testing.T)         { under(t).BooksAreCanonical(t) }
func TestCases(t *testing.T)                     { under(t).Cases(t) }

func TestPricesAreTheSameEverywhere(t *testing.T) {
	regions, err := Regions()
	if err != nil {
		t.Fatal(err)
	}
	if len(regions) != 0 {
		t.Errorf("ConoHa prices are all \"*\", got regions %v", regions)
	}
}
