package aws

import "github.com/O6lvl4/archgopher/flow"

// FlowConfig says which AWS types flow reads as queues, consumers and
// connection-limited stores.
func FlowConfig() flow.Config {
	return flow.Config{
		Queues: map[string]bool{"aws_sqs_queue": true},
		Consumers: map[string]flow.Consumer{
			"aws_lambda_function": {Caps: []string{"event_source_max_concurrency", "reserved_concurrent_executions"}, Duration: "durationMs"},
		},
		Limited: map[string]bool{
			"aws_rds_cluster": true, "aws_db_instance": true, "aws_db_proxy": true, "aws_docdb_cluster": true,
			"aws_neptune_cluster": true, "aws_redshift_cluster": true, "aws_elasticache_cluster": true,
			"aws_elasticache_replication_group": true, "aws_memorydb_cluster": true, "aws_mq_broker": true,
		},
	}
}
