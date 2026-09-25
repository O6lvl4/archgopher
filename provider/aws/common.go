package aws

import (
	"github.com/O6lvl4/archgopher/terraform/eval"
	"github.com/O6lvl4/archgopher/terraform/infer"
)

// roleAttrs are attribute paths through which a resource acts as an IAM role.
var roleAttrs = []string{"role", "role_arn", "task_role_arn"}

// knownTypes become nodes even without a scouter, so load keeps flowing and
// the report lists them as not read yet.
var knownTypes = []string{
	"aws_appsync_graphql_api", "aws_autoscaling_group", "aws_batch_job_queue",
	"aws_db_instance", "aws_efs_file_system", "aws_elasticache_cluster",
	"aws_elasticache_replication_group", "aws_glue_job", "aws_instance", "aws_kinesis_firehose_delivery_stream",
	"aws_kinesis_stream", "aws_lb", "aws_mq_broker", "aws_msk_cluster", "aws_opensearch_domain",
}

// common holds the rules that belong to no single service: IAM and KMS
// references, resources without a scouter yet, and where the region lives.
func common() infer.Rules {
	nodes := map[string]bool{}
	for _, t := range knownTypes {
		nodes[t] = true
	}
	return infer.Rules{
		NodeTypes: nodes,
		Links: []infer.Link{
			{Type: "aws_lb_target_group_attachment", From: "target_group_arn", To: []string{"target_id"}},
		},
		Aliases:    map[string]string{"aws_lb_listener": "load_balancer_arn"},
		FrontDoors: map[string]bool{"aws_lb": true, "aws_appsync_graphql_api": true},
		IgnoreRefs: append([]string{
			"execution_role_arn", "kms_key_arn", "kms_key_id", "kms_master_key_id", "policy",
			"server_side_encryption", "encryption_configuration",
		}, roleAttrs...),
		Region: func(ev *eval.Evaluated) string {
			r, _ := ev.Providers["aws"]["region"].(string)
			return r
		},
	}
}
