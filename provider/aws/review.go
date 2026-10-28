package aws

import "github.com/O6lvl4/archgopher/review"

const (
	lens          = "https://docs.aws.amazon.com/wellarchitected/latest/serverless-applications-lens/welcome.html"
	reliability   = "https://docs.aws.amazon.com/wellarchitected/latest/reliability-pillar/welcome.html"
	operational   = "https://docs.aws.amazon.com/wellarchitected/latest/operational-excellence-pillar/welcome.html"
	security      = "https://docs.aws.amazon.com/wellarchitected/latest/security-pillar/welcome.html"
	cost          = "https://docs.aws.amazon.com/wellarchitected/latest/cost-optimization-pillar/welcome.html"
	sqsLambda     = "https://docs.aws.amazon.com/lambda/latest/dg/with-sqs.html"
	sqsDLQ        = "https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-dead-letter-queues.html"
	foundational  = "https://docs.aws.amazon.com/securityhub/latest/userguide/fsbp-standard.html"
	priorityQueue = "https://learn.microsoft.com/en-us/azure/architecture/patterns/priority-queue"
)

func atLeast(n float64) *float64 { return &n }

var (
	sqsESM = review.Scope{Via: []review.By{{Type: "aws_sqs_queue", Path: "event_source_arn"}}}
	// deadLetter are the references that make a queue a place failures land.
	deadLetter = []review.By{
		{Type: "aws_sqs_queue", Path: "redrive_policy"},
		{Type: "aws_sqs_queue_redrive_policy", Path: "redrive_policy"},
		{Type: "aws_lambda_function_event_invoke_config", Path: "destination_config.on_failure"},
		{Type: "aws_lambda_event_source_mapping", Path: "destination_config.on_failure"},
	}
)

// ReviewRules are the AWS practices `review` reads the configuration against.
// Each one is judged from Terraform alone; what needs the running account
// (traffic, the bill, findings of Security Hub) is left to those tools.
func ReviewRules() []review.Rule {
	return []review.Rule{
		// Event-driven shape.
		{
			ID: "aws.lambda.one-queue-per-consumer", Pillar: "performance efficiency (Priority Queue pattern)", Source: priorityQueue,
			Title: "one Lambda function consumes several SQS queues",
			Hint: "Several queues into one function is the Priority Queue pattern only when the priorities are a requirement and each queue keeps its own consumer capacity " +
				"(a maximum concurrency per event source): name the pattern and the requirement. Otherwise use one queue and tell kinds of work apart by a message attribute.",
			FanIn: &review.FanIn{Node: "aws_lambda_function", From: "aws_sqs_queue", Max: 1},
		},
		{
			ID: "aws.sqs.dead-letter-queue", Pillar: "reliability", Source: sqsDLQ,
			Title: "queue without a dead-letter queue",
			Hint:  "A message that keeps failing should leave the queue for one where someone looks. Queues that are themselves where failures land are not asked.",
			Setting: &review.Setting{Type: "aws_sqs_queue", Path: "redrive_policy", Set: true,
				Scope: review.Scope{Except: deadLetter},
				Or:    []review.By{{Type: "aws_sqs_queue_redrive_policy", Path: "queue_url"}}},
		},
		{
			ID: "aws.sqs.dead-letter-alarm", Pillar: "operational excellence", Source: operational,
			Title: "dead-letter queue nobody is told about",
			Hint:  "Alarm on ApproximateNumberOfMessagesVisible of every queue failures land in; a dead-letter queue nobody watches loses the failures quietly.",
			Companion: &review.Companion{Type: "aws_sqs_queue", Scope: review.Scope{Only: deadLetter},
				By: []review.By{{Type: "aws_cloudwatch_metric_alarm", Path: "dimensions"}}, NameAttr: "name"},
		},
		{
			ID: "aws.sqs.visibility-timeout", Pillar: "Serverless Lens / reliability", Source: sqsLambda,
			Title: "queue's visibility timeout under six times its consumer's timeout",
			Hint:  "Lambda's guide sets the source queue's visibility timeout to at least six times the function's timeout, so a batch is not delivered again while it is still being worked on or retried after throttling.",
			Ratio: &review.Ratio{Type: "aws_lambda_event_source_mapping", Scope: sqsESM, Min: 6,
				Left:  review.End{Ref: "event_source_arn", Attr: "visibility_timeout_seconds", Default: 30},
				Right: review.End{Ref: "function_name", Attr: "timeout", Default: 3}},
		},
		{
			ID: "aws.lambda.sqs-partial-batch", Pillar: "Serverless Lens / reliability", Source: sqsLambda,
			Title: "SQS batch without partial batch responses",
			Hint:  "With more than one message per batch, report the failed ones (ReportBatchItemFailures); otherwise one bad message sends the whole batch back.",
			Setting: &review.Setting{Type: "aws_lambda_event_source_mapping", Scope: sqsESM, Path: "function_response_types", Contains: "ReportBatchItemFailures",
				WhenPath: "batch_size", WhenAbove: 1, WhenDefault: 10},
		},
		{
			ID: "aws.lambda.sqs-maximum-concurrency", Pillar: "Serverless Lens / performance efficiency", Source: sqsLambda,
			Title:   "SQS event source without a maximum concurrency",
			Hint:    "Cap how many functions the queue starts (scaling_config.maximum_concurrency) so a backlog does not flood what the function calls, such as a database.",
			Setting: &review.Setting{Type: "aws_lambda_event_source_mapping", Scope: sqsESM, Path: "scaling_config.maximum_concurrency", Set: true},
		},
		// Operations.
		{
			ID: "aws.lambda.error-alarm", Pillar: "operational excellence", Source: operational,
			Title: "Lambda function without an alarm",
			Hint: "Alarm on the function's Errors (or on a metric filter over its log) so a failing function is noticed without anyone looking. " +
				"Functions a queue, a state machine or an asynchronous failure destination stands behind are not asked: their failures surface there, and those have rules of their own.",
			Companion: &review.Companion{Type: "aws_lambda_function", Scope: review.Scope{Except: []review.By{
				{Type: "aws_lambda_event_source_mapping", Path: "function_name"},
				{Type: "aws_sfn_state_machine", Path: "definition"},
				{Type: "aws_lambda_function_event_invoke_config", Path: "function_name"},
			}},
				By: []review.By{{Type: "aws_cloudwatch_metric_alarm", Path: "dimensions"}}, NameAttr: "function_name"},
		},
		{
			ID: "aws.sfn.failure-alarm", Pillar: "operational excellence", Source: operational,
			Title: "state machine whose failures nobody is told about",
			Hint:  "Alarm on ExecutionsFailed and ExecutionsTimedOut, or route the execution status change event to a notification.",
			Companion: &review.Companion{Type: "aws_sfn_state_machine",
				By: []review.By{{Type: "aws_cloudwatch_metric_alarm", Path: "dimensions"}, {Type: "aws_cloudwatch_event_rule", Path: "event_pattern"}}, NameAttr: "name"},
		},
		{
			ID: "aws.sfn.logging", Pillar: "operational excellence", Source: foundational,
			Title:   "state machine without logging",
			Hint:    "Send execution history to CloudWatch Logs (logging_configuration) so a failed run can be traced after the console's history expires.",
			Setting: &review.Setting{Type: "aws_sfn_state_machine", Path: "logging_configuration.log_destination", Set: true},
		},
		{
			ID: "aws.logs.retention", Pillar: "cost optimization", Source: cost,
			Title:   "log group kept forever",
			Hint:    "Set retention_in_days; logs without it are stored and billed indefinitely.",
			Setting: &review.Setting{Type: "aws_cloudwatch_log_group", Path: "retention_in_days", Set: true},
		},
		{
			ID: "aws.iam.role-per-function", Pillar: "security", Source: security,
			Title:  "one IAM role behind several Lambda functions",
			Hint:   "Give each function its own role with only what it needs; a shared role grants every function the union of their permissions.",
			Shared: &review.Shared{Type: "aws_iam_role", By: review.By{Type: "aws_lambda_function", Path: "role"}},
		},
		// Data.
		{
			ID: "aws.kms.rotation", Pillar: "security", Source: foundational,
			Title:   "customer managed key without rotation",
			Hint:    "Turn on enable_key_rotation.",
			Setting: &review.Setting{Type: "aws_kms_key", Path: "enable_key_rotation", Equals: true, Default: false},
		},
		{
			ID: "aws.rds.deletion-protection", Pillar: "reliability", Source: foundational,
			Title:   "database cluster without deletion protection",
			Hint:    "Turn on deletion_protection where losing the data matters; a development cluster that is rebuilt from code may leave it off on purpose.",
			Setting: &review.Setting{Type: "aws_rds_cluster", Path: "deletion_protection", Equals: true, Default: false},
		},
		{
			ID: "aws.rds.backup-retention", Pillar: "reliability", Source: foundational,
			Title:   "database cluster keeps backups under 7 days",
			Hint:    "Keep automated backups at least 7 days (backup_retention_period) where the data cannot be loaded again.",
			Setting: &review.Setting{Type: "aws_rds_cluster", Path: "backup_retention_period", AtLeast: atLeast(7), Default: 1},
		},
		{
			ID: "aws.rds.storage-encrypted", Pillar: "security", Source: foundational,
			Title:   "database cluster storage not encrypted",
			Hint:    "Set storage_encrypted; it cannot be turned on later without a restore.",
			Setting: &review.Setting{Type: "aws_rds_cluster", Path: "storage_encrypted", Equals: true, Default: false},
		},
		{
			ID: "aws.dynamodb.point-in-time-recovery", Pillar: "reliability", Source: foundational,
			Title:   "DynamoDB table without point-in-time recovery",
			Hint:    "Turn on point_in_time_recovery unless the table is rebuilt from another source of truth.",
			Setting: &review.Setting{Type: "aws_dynamodb_table", Path: "point_in_time_recovery.enabled", Equals: true, Default: false},
		},
		{
			ID: "aws.s3.public-access-block", Pillar: "security", Source: foundational,
			Title: "bucket without a public access block",
			Hint:  "Add aws_s3_bucket_public_access_block with all four settings on.",
			Companion: &review.Companion{Type: "aws_s3_bucket",
				By: []review.By{{Type: "aws_s3_bucket_public_access_block", Path: "bucket"}}, NameAttr: "bucket"},
		},
		{
			ID: "aws.ecr.scan-on-push", Pillar: "security", Source: foundational,
			Title: "repository images not scanned",
			Hint:  "Scan on push, or scan the whole registry (aws_ecr_registry_scanning_configuration).",
			Setting: &review.Setting{Type: "aws_ecr_repository", Path: "image_scanning_configuration.scan_on_push", Equals: true, Default: false,
				Scope: review.Scope{UnlessAny: []string{"aws_ecr_registry_scanning_configuration"}}},
		},
		{
			ID: "aws.ecr.tag-immutability", Pillar: "security", Source: foundational,
			Title:   "repository tags can be overwritten",
			Hint:    "Set image_tag_mutability to IMMUTABLE (or IMMUTABLE_WITH_EXCLUSION, naming the tags that may move) so a tag always names the same image.",
			Setting: &review.Setting{Type: "aws_ecr_repository", Path: "image_tag_mutability", OneOf: []any{"IMMUTABLE", "IMMUTABLE_WITH_EXCLUSION"}, Default: "MUTABLE"},
		},
		{
			ID: "aws.ecr.lifecycle", Pillar: "cost optimization", Source: foundational,
			Title: "repository keeps every image",
			Hint:  "Add aws_ecr_lifecycle_policy to expire old images.",
			Companion: &review.Companion{Type: "aws_ecr_repository",
				By: []review.By{{Type: "aws_ecr_lifecycle_policy", Path: "repository"}}, NameAttr: "name"},
		},
	}
}
