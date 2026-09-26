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
		"azurerm_service_plan":                      {"sku_name": "P1v3"},
		"azurerm_cognitive_deployment":              {"model_name": "gpt-4o"},
		"azurerm_linux_virtual_machine":             {"size": "Standard_D2s_v5", "os_disk_type": "Premium_LRS"},
		"azurerm_windows_virtual_machine":           {"size": "Standard_D2s_v5", "os_disk_type": "Premium_LRS"},
		"azurerm_virtual_machine":                   {"vm_size": "Standard_D2s_v5", "os_disk_type": "Premium_LRS"},
		"azurerm_linux_virtual_machine_scale_set":   {"sku": "Standard_D2s_v5", "instances": 2.0, "os_disk_type": "Premium_LRS"},
		"azurerm_windows_virtual_machine_scale_set": {"sku": "Standard_D2s_v5", "instances": 2.0, "os_disk_type": "Premium_LRS"},
		"azurerm_virtual_machine_scale_set":         {"sku_name": "Standard_D2s_v5", "sku_capacity": 2.0, "os_disk_type": "Premium_LRS"},
		"azurerm_managed_disk":                      {"storage_account_type": "StandardSSD_LRS", "disk_size_gb": 128.0},
		"azurerm_snapshot":                          {"disk_size_gb": 128.0},
		"azurerm_image":                             {"os_disk_size_gb": 30.0},
		"azurerm_kubernetes_cluster":                {"vm_size": "Standard_D2s_v5", "sku_tier": "Standard"},
		"azurerm_kubernetes_cluster_node_pool":      {"vm_size": "Standard_D2s_v5"},
		"azurerm_lb":                                {"sku": "Standard"},
		"azurerm_firewall_policy":                   {"insights": true},
		"azurerm_storage_management_policy": {
			"coolAfterModification": 30.0, "coldAfterModification": 90.0, "archiveAfterModification": 180.0,
		},
		"azurerm_monitor_action_group":                   {"email_receivers": 1.0, "push_receivers": 1.0, "itsm_receivers": 1.0, "webhook_receivers": 2.0, "secure_webhook_receivers": 1.0, "sms_receivers": 1.0, "voice_receivers": 1.0},
		"azurerm_monitor_diagnostic_setting":             {"storage_account_id": true},
		"azurerm_monitor_data_collection_rule":           {"metrics_destination": true},
		"azurerm_monitor_metric_alert":                   {"dynamic_criteria": 1.0},
		"azurerm_monitor_scheduled_query_rules_alert_v2": {"scopes": 2.0},
		"azurerm_log_analytics_solution":                 {"solution_name": "SecurityInsights"},
		"azurerm_redis_cache":                            {"sku_name": "Premium", "family": "P", "capacity": 1.0, "shard_count": 2.0},
		"azurerm_managed_redis":                          {"sku_name": "Balanced_B5"},
		"azurerm_search_service":                         {"sku": "standard", "semantic_search_sku": "standard"},
	}
	assume = map[string]map[string]any{
		"azurerm_cosmosdb_account": {"provisionedRus": 400.0},
		// Network nodes that load passes through need the data each unit carries.
		"azurerm_virtual_network_gateway":   {"kbPerUnit": 4.0},
		"azurerm_vpn_gateway":               {"kbPerUnit": 4.0},
		"azurerm_point_to_site_vpn_gateway": {"kbPerUnit": 4.0},
		"azurerm_express_route_gateway":     {"kbPerUnit": 4.0},
		"azurerm_virtual_hub":               {"kbPerUnit": 4.0},
		"azurerm_lb":                        {"kbPerUnit": 4.0},
		"azurerm_lb_rule":                   {"beyondFirstFive": true},
		"azurerm_lb_outbound_rule":          {"beyondFirstFive": true},
		"azurerm_application_gateway":       {"kbPerUnit": 8.0},
		"azurerm_nat_gateway":               {"kbPerUnit": 4.0},
		"azurerm_firewall":                  {"kbPerUnit": 4.0},
		"azurerm_firewall_policy":           {"firewalls": 2.0},
		"azurerm_private_endpoint":          {"kbPerUnit": 4.0},
		"azurerm_virtual_network_peering":   {"kbPerUnit": 4.0},
		"azurerm_storage_share":             {"storageGb": 1.0},
		"azurerm_storage_management_policy": {"blobsAged": 1.0},
		"azurerm_redis_cache":               {"datasetGb": 4.0, "peakConnections": 500.0},
		"azurerm_managed_redis":             {"datasetGb": 4.0, "peakConnections": 500.0},
		"azurerm_search_service":            {"imagesMonthly": 6e6, "indexGb": 10.0},
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
		Dir: filepath.Join("..", "..", "catalog", "azure"), Units: cat.MustUnits(), Books: books, Registry: Registry(), Regions: regions,
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

func TestAReferenceSetsABooleanThatPointsAtIt(t *testing.T) {
	ev, err := eval.Evaluate(filepath.Join("testdata", "sql"), eval.Options{})
	if err != nil {
		t.Fatal(err)
	}
	spec, _ := infer.Build(ev, TerraformRules(), "sql")
	pooled, single, empty := map[string]any(nil), map[string]any(nil), map[string]any(nil)
	for _, n := range spec.Nodes {
		switch n.ID {
		case "pooled":
			pooled = n.Attributes
		case "single":
			single = n.Attributes
		case "empty":
			empty = n.Attributes
		}
	}
	if pooled["elastic_pool"] != true {
		t.Errorf("elastic_pool_id refers to the pool, so the database is in it: %v", pooled)
	}
	if _, set := single["elastic_pool"]; set || single["sku_name"] != "S0" {
		t.Errorf("a database without elastic_pool_id is not in a pool: %v", single)
	}
	if _, set := empty["elastic_pool"]; set {
		t.Errorf("an empty elastic_pool_id is not a pool: %v", empty)
	}
	if len(spec.Edges) != 0 {
		t.Errorf("databases call nothing: %v", spec.Edges)
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
