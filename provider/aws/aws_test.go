package aws

import (
	"flag"
	"path/filepath"
	"strings"
	"testing"

	"github.com/O6lvl4/archgopher/internal/catalogtest"
)

// attrs makes each scouter take its main code path.
var attrs = map[string]map[string]any{
	"aws_rds_cluster":         {"max_capacity": 16.0, "min_capacity": 0.5},
	"aws_dynamodb_table":      {"billing_mode": "PAY_PER_REQUEST"},
	"aws_sfn_state_machine":   {"type": "STANDARD"},
	"aws_ecs_task_definition": {"cpu": 256.0, "memory": 512.0},
	"aws_db_instance": {
		"engine": "postgres", "instance_class": "db.t3.micro", "multi_az": true, "storage_type": "gp3",
		"allocated_storage": 500.0, "iops": 15000.0, "storage_throughput": 600.0,
		"performance_insights_enabled": true, "performance_insights_retention_period": 93.0,
	},
	"aws_rds_cluster_instance":          {"engine": "aurora-postgresql", "instance_class": "db.t4g.medium", "performance_insights_enabled": true, "performance_insights_retention_period": 31.0},
	"aws_docdb_cluster_instance":        {"instance_class": "db.t3.medium"},
	"aws_neptune_cluster_instance":      {"instance_class": "db.t3.medium"},
	"aws_elasticache_cluster":           {"engine": "redis", "node_type": "cache.t4g.micro", "snapshot_retention_limit": 3.0},
	"aws_elasticache_replication_group": {"engine": "valkey", "node_type": "cache.r7g.large", "num_node_groups": 2.0, "replicas_per_node_group": 1.0, "snapshot_retention_limit": 3.0},
	"aws_redshift_cluster":              {"node_type": "ra3.xlplus", "number_of_nodes": 2.0},
	"aws_dms_replication_instance":      {"replication_instance_class": "dms.t3.medium", "multi_az": true, "allocated_storage": 100.0},
	// Required in Terraform, so they have no default.
	"aws_lambda_provisioned_concurrency_config": {"provisioned_concurrent_executions": 5.0},
	"aws_sns_topic_subscription":                {"protocol": "https"},
	// A Direct Connect location is priced in its home region only.
	"aws_dx_connection":                          {"bandwidth": "1Gbps", "location": "EqTY2"},
	"aws_ec2_transit_gateway_peering_attachment": {"peer_region": "us-east-1"},
}

var assume = map[string]map[string]any{
	"aws_rds_cluster":       {"ioPerQuery": 2.0, "peakAcu": 4.0, "averageAcu": 2.0},
	"aws_sfn_state_machine": {"transitionsPerExecution": 5.0},
	// Invocations routed to provisioned concurrency need their duration.
	"aws_lambda_provisioned_concurrency_config": {"durationMs": 100.0},
	// Operations whose edges give no size take the table's item size.
	"aws_dynamodb_table": {"itemSizeKb": 1.0},
	// Network nodes that load passes through need the data each unit carries.
	"aws_ec2_transit_gateway_vpc_attachment": {"kbPerUnit": 4.0},
	"aws_vpc_endpoint":                       {"kbPerUnit": 4.0},
	"aws_vpn_connection":                     {"kbPerUnit": 4.0},
	"aws_nat_gateway":                        {"kbPerUnit": 4.0},
	// Databases: every per-vCPU, backup and headroom line.
	"aws_db_instance":                            {"vcpus": 2.0, "ioPerQuery": 1.0, "backupGb": 10.0, "cpuCreditVcpuHours": 5.0, "extendedSupport": "year1_2", "insightsApiCalls": 1000.0},
	"aws_rds_cluster_instance":                   {"vcpus": 2.0, "cpuCreditVcpuHours": 5.0, "extendedSupport": "year3"},
	"aws_docdb_cluster":                          {"ioPerQuery": 2.0, "backupGb": 10.0},
	"aws_neptune_cluster":                        {"ioPerQuery": 2.0, "backupGb": 10.0},
	"aws_docdb_cluster_instance":                 {"cpuCreditVcpuHours": 5.0},
	"aws_neptune_cluster_instance":               {"cpuCreditVcpuHours": 5.0},
	"aws_elasticache_cluster":                    {"snapshotGb": 1.0, "datasetGb": 0.2},
	"aws_elasticache_replication_group":          {"snapshotGb": 5.0, "datasetGb": 10.0},
	"aws_redshift_cluster":                       {"concurrencyScalingSeconds": 100.0, "spectrumTb": 1.0, "backupGb": 60000.0},
	"aws_ec2_transit_gateway_peering_attachment": {"kbPerUnit": 4.0},
	"aws_dx_connection":                          {"kbPerUnit": 4.0},
	"aws_dx_gateway_association":                 {"kbPerUnit": 4.0},
	"aws_networkfirewall_firewall":               {"kbPerUnit": 4.0},
	"aws_ec2_client_vpn_endpoint":                {"clients": 10.0},
	"aws_flow_log":                               {"gbPerMonth": 10.0},
}

// TestRegionalQuotas pins quotas that differ by region, read from each
// service's quota page: named regions override the "*" default.
func TestRegionalQuotas(t *testing.T) {
	books, err := Books()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		id, region string
		want       float64
	}{
		{"aws.sns.publish_rps", "us-east-1", 30000},
		{"aws.sns.publish_rps", "eu-west-1", 9000},
		{"aws.sns.publish_rps", "ap-northeast-1", 1500},
		{"aws.sns.publish_rps", "sa-east-1", 300},
		{"aws.sfn.standard.start_execution", "us-west-2", 300},
		{"aws.sfn.standard.start_execution", "eu-central-1", 150},
		{"aws.agentcore.runtime.active_sessions", "us-west-2", 5000},
		{"aws.agentcore.runtime.active_sessions", "ap-northeast-1", 2500},
		{"aws.agentcore.evaluation.per_minute", "ap-southeast-1", 200},
		{"aws.agentcore.evaluation.per_minute", "eu-west-1", 1200},
		{"aws.sqs.fifo.high_throughput_tps", "us-west-2", 70000},
		{"aws.sqs.fifo.high_throughput_tps", "eu-central-1", 19000},
		{"aws.sqs.fifo.high_throughput_tps", "ap-northeast-1", 9000},
		{"aws.sqs.fifo.high_throughput_tps", "ap-northeast-2", 2400},
		{"aws.sqs.fifo.tps", "ap-northeast-2", 300},
		{"aws.apigateway.account_rps", "ap-northeast-1", 10000},
		{"aws.apigateway.account_rps", "ap-southeast-4", 2500},
	} {
		_, v, err := books.Quotas.Lookup(c.id, c.region)
		if err != nil || v.Value == nil || *v.Value != c.want {
			t.Errorf("%s in %s: want %v, got %v (%v)", c.id, c.region, c.want, show(v.Value), err)
		}
	}
}

func show(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func TestIAMKinds(t *testing.T) {
	cases := []struct {
		target  string
		actions []string
		want    string
	}{
		{"aws_dynamodb_table", []string{"dynamodb:GetItem", "dynamodb:PutItem"}, "read,write"},
		{"aws_dynamodb_table", []string{"dynamodb:*"}, "read,write"},
		{"aws_dynamodb_table", []string{"dynamodb:Get*"}, "read"},
		{"aws_dynamodb_table", []string{"dynamodb:DescribeTable"}, ""},
		{"aws_s3_bucket", []string{"s3:PutObject"}, "write"},
		{"aws_sqs_queue", []string{"sqs:ReceiveMessage"}, ""},
		{"aws_sqs_queue", nil, ""},
		{"aws_kinesis_stream", []string{"kinesis:PutRecord"}, ""},
	}
	for _, c := range cases {
		got := strings.Join(IAM().Kinds(c.target, c.actions), ",")
		if got != c.want {
			t.Errorf("%s %v: got %q, want %q", c.target, c.actions, got, c.want)
		}
	}
}

var update = flag.Bool("update", false, "rewrite the bundled books in canonical form")

// A row belongs to exactly one unit.
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
		Dir: filepath.Join("..", "..", "catalog", "aws"), Units: mustUnits(), Books: books, Registry: Registry(), Regions: regions,
		Attrs: attrs, Assume: assume, Update: *update, UpdateHint: "go test ./provider/aws -update",
	}
}

func TestEveryScouterReadsTheBooks(t *testing.T) { under(t).EveryScouterReadsTheBooks(t) }
func TestEveryRegionIsComplete(t *testing.T)     { under(t).EveryRegionIsComplete(t) }
func TestBooksAreWellFormed(t *testing.T)        { under(t).BooksAreWellFormed(t) }
func TestBooksAreCanonical(t *testing.T)         { under(t).BooksAreCanonical(t) }
func TestCases(t *testing.T)                     { under(t).Cases(t) }

func TestUnitsDoNotShareIDs(t *testing.T) {
	if _, err := Books(); err != nil {
		t.Fatal(err)
	}
}
