package azure

import (
	"flag"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/O6lvl4/archgopher/internal/catalogtest"
	"github.com/O6lvl4/archgopher/terraform/eval"
	"github.com/O6lvl4/archgopher/terraform/infer"
)

var update = flag.Bool("update", false, "rewrite the bundled books in canonical form and the cases' expected values")

// attrs and assume make each resource take its main code path.
var (
	attrs = map[string]map[string]any{
		"azurerm_service_plan":         {"sku_name": "P1v3"},
		"azurerm_cognitive_deployment": {"model_name": "gpt-4o"},
		"azurerm_redis_cache":          {"sku_name": "Premium", "family": "P", "capacity": 1.0, "shard_count": 2.0},
		"azurerm_managed_redis":        {"sku_name": "Balanced_B5"},
		"azurerm_search_service":       {"sku": "standard", "semantic_search_sku": "standard"},
	}
	assume = map[string]map[string]any{
		"azurerm_cosmosdb_account": {"provisionedRus": 400.0},
		"azurerm_redis_cache":      {"datasetGb": 4.0, "peakConnections": 500.0},
		"azurerm_managed_redis":    {"datasetGb": 4.0, "peakConnections": 500.0},
		"azurerm_search_service":   {"imagesMonthly": 6e6, "indexGb": 10.0},
	}
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

func TestRoleAssignmentsBecomeEdges(t *testing.T) {
	ev, err := eval.Evaluate(filepath.Join("testdata", "rbac"), eval.Options{})
	if err != nil {
		t.Fatal(err)
	}
	spec, _ := infer.Build(ev, TerraformRules(), "rbac")
	if spec.Region != "japaneast" {
		t.Errorf("region: %q", spec.Region)
	}
	got := map[string]bool{}
	for _, e := range spec.Edges {
		got[e.From+">"+e.To+":"+e.Kind] = true
	}
	for _, want := range []string{"api>data:read", "api>bus:send"} {
		if !got[want] {
			t.Errorf("want edge %s, got %v", want, spec.Edges)
		}
	}
}

func TestCallsToCosmosDBContainersReachTheAccount(t *testing.T) {
	ev, err := eval.Evaluate(filepath.Join("testdata", "cosmos"), eval.Options{})
	if err != nil {
		t.Fatal(err)
	}
	spec, _ := infer.Build(ev, TerraformRules(), "cosmos")
	var got []string
	for _, e := range spec.Edges {
		got = append(got, e.From+">"+e.To)
	}
	sort.Strings(got)
	// The API names a container and a Cassandra table: each is a node with its
	// own throughput, and the calls also reach the account (through the
	// keyspace for the table). A database read as a data source is no node but
	// still reaches its account. Containers and keyspaces call nothing themselves.
	want := "api>archive api>events api>log api>orders api>plan api>shop api>store users>api"
	if strings.Join(got, " ") != want {
		t.Errorf("edges: want %s, got %s", want, strings.Join(got, " "))
	}
}
