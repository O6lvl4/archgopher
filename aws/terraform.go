package aws

import (
	"path"
	"strings"

	"github.com/O6lvl4/arch-scouter/terraform"
)

// knownTypes become nodes even without a scouter, so load keeps flowing
// and the report lists them as not read yet.
var knownTypes = []string{
	"aws_appsync_graphql_api", "aws_autoscaling_group", "aws_batch_job_queue", "aws_bedrockagentcore_agent_runtime",
	"aws_cognito_user_pool", "aws_db_instance", "aws_ecs_service", "aws_efs_file_system", "aws_elasticache_cluster",
	"aws_elasticache_replication_group", "aws_glue_job", "aws_instance", "aws_kinesis_firehose_delivery_stream",
	"aws_kinesis_stream", "aws_lb", "aws_mq_broker", "aws_msk_cluster", "aws_opensearch_domain",
}

// TerraformRules describe how AWS resources form a load graph.
func TerraformRules() terraform.Rules {
	reg := Registry()
	nodes := map[string]bool{}
	for t, s := range reg {
		if !s.Meta().External {
			nodes[t] = true
		}
	}
	for _, t := range knownTypes {
		nodes[t] = true
	}
	return terraform.Rules{
		NodeTypes: nodes,
		Scouters:  reg,
		Links: []terraform.Link{
			{Type: "aws_api_gateway_integration", From: "rest_api_id", To: []string{"uri"}},
			{Type: "aws_apigatewayv2_integration", From: "api_id", To: []string{"integration_uri"}},
			{Type: "aws_lambda_event_source_mapping", From: "event_source_arn", To: []string{"function_name"}},
			{Type: "aws_sns_topic_subscription", From: "topic_arn", To: []string{"endpoint"}},
			{Type: "aws_cloudwatch_event_target", From: "rule", To: []string{"arn"}},
			{Type: "aws_s3_bucket_notification", From: "bucket", To: []string{"lambda_function.lambda_function_arn", "queue.queue_arn", "topic.topic_arn"}},
			{Type: "aws_pipes_pipe", From: "source", To: []string{"target"}},
			{Type: "aws_lb_target_group_attachment", From: "target_group_arn", To: []string{"target_id"}},
		},
		Aliases: map[string]string{
			"aws_lambda_alias":                    "function_name",
			"aws_api_gateway_stage":               "rest_api_id",
			"aws_api_gateway_deployment":          "rest_api_id",
			"aws_apigatewayv2_stage":              "api_id",
			"aws_s3_bucket_website_configuration": "bucket",
			"aws_lb_listener":                     "load_balancer_arn",
		},
		FrontDoors: map[string]bool{
			"aws_cloudfront_distribution": true, "aws_api_gateway_rest_api": true, "aws_apigatewayv2_api": true,
			"aws_lb": true, "aws_appsync_graphql_api": true, "aws_cognito_user_pool": true,
		},
		FrontDoorAliases: map[string]string{"aws_lambda_function_url": "function_name"},
		Mentioned:        map[string]bool{"aws_cloudfront_distribution": true},
		Schedules: map[string]string{
			"aws_scheduler_schedule":    "schedule_expression",
			"aws_cloudwatch_event_rule": "schedule_expression",
		},
		RoleAttrs: []string{"role", "role_arn", "task_role_arn"},
		IgnoreRefs: []string{
			"execution_role_arn", "kms_key_arn", "kms_key_id", "kms_master_key_id", "vpc_config", "layers",
			"dead_letter_config", "redrive_policy", "redrive_allow_policy", "logging_config", "tracing_config",
			"web_acl_id", "viewer_certificate", "policy", "server_side_encryption", "encryption_configuration",
			"target.dead_letter_config", "target.retry_policy", "target.role_arn", "event_bus_name", "replica",
		},
		RoleLinks: []terraform.RoleLink{
			{Type: "aws_iam_role_policy", Role: "role"},
			{Type: "aws_iam_role_policy_attachment", Role: "role", Policy: "policy_arn"},
			{Type: "aws_iam_policy_attachment", Role: "roles", Policy: "policy_arn"},
		},
		Kinds: iamKinds,
	}
}

// actionKinds maps the IAM actions that carry load to kinds of work, per target type.
var actionKinds = map[string]map[string][]string{
	"aws_dynamodb_table": {
		"read":  {"dynamodb:GetItem", "dynamodb:BatchGetItem", "dynamodb:Query", "dynamodb:Scan", "dynamodb:ConditionCheckItem"},
		"write": {"dynamodb:PutItem", "dynamodb:UpdateItem", "dynamodb:DeleteItem", "dynamodb:BatchWriteItem"},
	},
	"aws_s3_bucket": {
		"read":  {"s3:GetObject"},
		"write": {"s3:PutObject", "s3:DeleteObject"},
	},
	"aws_sqs_queue":         {"send": {"sqs:SendMessage"}},
	"aws_sns_topic":         {"publish": {"sns:Publish"}},
	"aws_lambda_function":   {"invoke": {"lambda:InvokeFunction"}},
	"aws_sfn_state_machine": {"execution": {"states:StartExecution", "states:StartSyncExecution"}},
	"aws_rds_cluster":       {"query": {"rds-data:ExecuteStatement", "rds-data:BatchExecuteStatement", "rds-db:connect"}},
}

// iamKinds returns the kinds a set of actions exercises on a target. Unknown
// actions give the target's default kind; known targets with no load-carrying
// action (DescribeTable, ListBucket) give no edge.
func iamKinds(targetType string, actions []string) []string {
	if actions == nil {
		return []string{""}
	}
	table, ok := actionKinds[targetType]
	if !ok {
		return []string{""}
	}
	var out []string
	for _, kind := range []string{"read", "write", "send", "publish", "invoke", "execution", "query"} {
		for _, known := range table[kind] {
			if anyMatch(actions, known) {
				out = append(out, kind)
				break
			}
		}
	}
	return out
}

func anyMatch(globs []string, action string) bool {
	for _, g := range globs {
		if ok, _ := path.Match(strings.ToLower(g), strings.ToLower(action)); ok {
			return true
		}
	}
	return false
}
