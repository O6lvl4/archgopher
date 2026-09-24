package azure

import (
	"flag"
	"path/filepath"
	"testing"

	"github.com/O6lvl4/archgopher/internal/catalogtest"
	"github.com/O6lvl4/archgopher/terraform/eval"
)

var update = flag.Bool("update", false, "rewrite the bundled books in canonical form and the cases' expected values")

// attrs and assume make each resource take its main code path.
var (
	attrs  = map[string]map[string]any{}
	assume = map[string]map[string]any{}
)

func under(t *testing.T) catalogtest.Catalog {
	t.Helper()
	books, err := Books()
	if err != nil {
		t.Fatal(err)
	}
	regions, err := Regions()
	if err != nil {
		t.Fatal(err)
	}
	return catalogtest.Catalog{
		Dir: filepath.Join("..", "..", "catalog", "azure"), Units: mustUnits(), Books: books, Registry: Registry(), Regions: regions,
		Attrs: attrs, Assume: assume, Update: *update, UpdateHint: "go test ./provider/azure -update",
	}
}

func TestEveryScouterReadsTheBooks(t *testing.T) { under(t).EveryScouterReadsTheBooks(t) }
func TestEveryRegionIsComplete(t *testing.T)     { under(t).EveryRegionIsComplete(t) }
func TestBooksAreWellFormed(t *testing.T)        { under(t).BooksAreWellFormed(t) }
func TestBooksAreCanonical(t *testing.T)         { under(t).BooksAreCanonical(t) }
func TestCases(t *testing.T)                     { under(t).Cases(t) }

func TestTenRegions(t *testing.T) {
	regions, err := Regions()
	if err != nil {
		t.Fatal(err)
	}
	if len(regions) != 10 {
		t.Errorf("the catalog covers ten regions, got %v", regions)
	}
}

func TestRegionIsTheMostCommonLocation(t *testing.T) {
	ev := &eval.Evaluated{Resources: []*eval.Resource{
		{Type: "azurerm_resource_group", Attrs: map[string]any{"location": "Japan East"}},
		{Type: "azurerm_storage_account", Attrs: map[string]any{"location": "japaneast"}},
		{Type: "azurerm_linux_function_app", Attrs: map[string]any{"location": "East US"}},
		{Type: "aws_s3_bucket", Attrs: map[string]any{"location": "westus"}},
	}}
	if got := Region(ev); got != "japaneast" {
		t.Errorf("want japaneast, got %q", got)
	}
}
