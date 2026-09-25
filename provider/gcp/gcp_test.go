package gcp

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
	attrs = map[string]map[string]any{
		"google_sql_database_instance": {"tier": "db-custom-2-7680", "database_version": "POSTGRES_16"},
	}
	assume = map[string]map[string]any{
		"google_sql_database_instance":             {"queryMs": 20.0},
		"google_compute_forwarding_rule":           {"kbPerUnit": 4.0},
		"google_compute_target_http_proxy":         {"kbPerUnit": 4.0},
		"google_compute_target_https_proxy":        {"kbPerUnit": 4.0},
		"google_compute_region_target_http_proxy":  {"kbPerUnit": 4.0},
		"google_compute_region_target_https_proxy": {"kbPerUnit": 4.0},
		"google_compute_target_ssl_proxy":          {"kbPerUnit": 4.0},
		"google_compute_target_tcp_proxy":          {"kbPerUnit": 4.0},
		"google_compute_router_nat":                {"kbPerUnit": 4.0},
		"google_compute_vpn_gateway":               {"kbPerUnit": 4.0},
		"google_compute_ha_vpn_gateway":            {"kbPerUnit": 4.0, "peer": "same-region"},
		"google_compute_external_vpn_gateway":      {"kbPerUnit": 4.0},
		"google_service_networking_connection":     {"kbPerUnit": 4.0},
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
		Dir: filepath.Join("..", "..", "catalog", "gcp"), Units: mustUnits(), Books: books, Registry: Registry(), Regions: regions,
		Attrs: attrs, Assume: assume, Update: *update, UpdateHint: "go test ./provider/gcp -update",
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

func TestRegionFromProviderOrResources(t *testing.T) {
	ev := &eval.Evaluated{Providers: map[string]map[string]any{"google": {"region": "asia-northeast1"}}}
	if got := Region(ev); got != "asia-northeast1" {
		t.Errorf("provider region: %q", got)
	}
	ev = &eval.Evaluated{Resources: []*eval.Resource{
		{Type: "google_storage_bucket", Attrs: map[string]any{"location": "ASIA-NORTHEAST1"}},
		{Type: "google_cloud_run_v2_service", Attrs: map[string]any{"location": "asia-northeast1"}},
		{Type: "google_storage_bucket", Attrs: map[string]any{"location": "US"}},
	}}
	if got := Region(ev); got != "asia-northeast1" {
		t.Errorf("resource locations: %q", got)
	}
}
