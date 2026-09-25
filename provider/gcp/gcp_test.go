package gcp

import (
	"flag"
	"path/filepath"
	"testing"

	"github.com/O6lvl4/archgopher/internal/catalogtest"
	"github.com/O6lvl4/archgopher/model"
	"github.com/O6lvl4/archgopher/terraform/eval"
	"github.com/O6lvl4/archgopher/terraform/infer"
)

var update = flag.Bool("update", false, "rewrite the bundled books in canonical form and the cases' expected values")

// attrs and assume make each resource take its main code path.
var (
	attrs = map[string]map[string]any{
		"google_sql_database_instance": {"tier": "db-custom-2-7680", "database_version": "POSTGRES_16"},
		"google_compute_instance": {"machine_type": "n1-standard-2", "gpu_type": "nvidia-tesla-t4", "gpu_count": 1.0, "scratch_disks": 1.0,
			"external_ip": true, "boot_disk_type": "hyperdisk-balanced", "boot_disk_size": 100.0},
		"google_compute_instance_group_manager":        {"machine_type": "n2-custom-4-40960-ext", "target_size": 2.0},
		"google_compute_region_instance_group_manager": {"machine_type": "e2-standard-2", "preemptible": true, "external_ip": true},
		"google_compute_per_instance_config":           {"machine_type": "c3-standard-4"},
		"google_compute_region_per_instance_config":    {"machine_type": "custom-2-8192"},
		"google_compute_disk":                          {"type": "hyperdisk-balanced", "size": 500.0},
		"google_compute_image":                         {"disk_size_gb": 10.0},
		"google_compute_snapshot":                      {"source_disk_size": 100.0},
		"google_container_cluster":                     {"enable_autopilot": true},
	}
	assume = map[string]map[string]any{
		"google_sql_database_instance":  {"queryMs": 20.0},
		"google_compute_machine_image":  {"storedGb": 20.0},
		"google_compute_global_address": {"use": "unused"},
		"google_container_cluster":      {"autopilotVcpu": 2.0, "autopilotMemoryGb": 8.0, "autopilotEphemeralGb": 10.0},
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

// Terraform import reads attributes through references ("ref->path"): an
// instance group reads its template, a per-instance config its group's
// template and a node pool its cluster; "blocks.#" counts blocks.
func TestTerraformReadsThroughReferences(t *testing.T) {
	ev, err := eval.Evaluate(filepath.Join("testdata", "compute"), eval.Options{})
	if err != nil {
		t.Fatal(err)
	}
	spec, _ := infer.Build(ev, TerraformRules(), "compute")
	nodes := map[string]model.Node{}
	for _, n := range spec.Nodes {
		nodes[n.Type] = n
	}
	want := map[string]map[string]any{
		"google_compute_instance_group_manager": {"machine_type": "n2-standard-4", "provisioning_model": "SPOT", "disk_type": "pd-balanced", "disk_size_gb": 20, "external_ip": true, "target_size": 3},
		"google_compute_per_instance_config":    {"machine_type": "n2-standard-4", "provisioning_model": "SPOT", "disk_size_gb": 20},
		"google_compute_instance":               {"machine_type": "n1-standard-2", "scratch_disks": 2},
		"google_container_node_pool":            {"machine_type": "e2-standard-4", "cluster_location": "asia-northeast1", "cluster_node_locations": 2, "node_count": 2},
		"google_container_cluster":              {"node_locations": 2, "remove_default_node_pool": true},
	}
	for typ, attrs := range want {
		n, ok := nodes[typ]
		if !ok {
			t.Errorf("no %s node", typ)
			continue
		}
		for k, v := range attrs {
			if got := n.Attributes[k]; got != v {
				t.Errorf("%s %s = %v (%T), want %v", typ, k, got, got, v)
			}
		}
	}
	var disk bool
	for _, e := range spec.Edges {
		if e.From == "db" && e.To == "data" {
			disk = true
		}
	}
	if !disk {
		t.Errorf("the instance should do I/O on its attached disk: %+v", spec.Edges)
	}
}
