# Each "bad" resource departs from one review rule; each "good" one meets it.
provider "aws" {
  region = "us-east-1"
}

# --- queues and their consumers ----------------------------------------------

resource "aws_sqs_queue" "dead" {
  name = "dead"
}

resource "aws_sqs_queue" "good" {
  name                       = "good"
  visibility_timeout_seconds = 360
  redrive_policy             = jsonencode({ deadLetterTargetArn = aws_sqs_queue.dead.arn, maxReceiveCount = 3 })
}

resource "aws_sqs_queue" "bad" {
  name = "bad"
}

resource "aws_sqs_queue" "unwatched_dead" {
  name = "unwatched-dead"
}

resource "aws_sqs_queue" "watched" {
  name           = "watched"
  redrive_policy = jsonencode({ deadLetterTargetArn = aws_sqs_queue.unwatched_dead.arn, maxReceiveCount = 3 })
}

resource "aws_cloudwatch_metric_alarm" "dead" {
  alarm_name          = "dead-messages"
  namespace           = "AWS/SQS"
  metric_name         = "ApproximateNumberOfMessagesVisible"
  dimensions          = { QueueName = aws_sqs_queue.dead.name }
  statistic           = "Maximum"
  period              = 300
  evaluation_periods  = 1
  threshold           = 1
  comparison_operator = "GreaterThanOrEqualToThreshold"
}

resource "aws_iam_role" "good" {
  name               = "good"
  assume_role_policy = "{}"
}

resource "aws_iam_role" "shared" {
  name               = "shared"
  assume_role_policy = "{}"
}

resource "aws_lambda_function" "good" {
  function_name = "good"
  role          = aws_iam_role.good.arn
  runtime       = "nodejs22.x"
  handler       = "index.handler"
  filename      = "good.zip"
  timeout       = 60
}

resource "aws_lambda_event_source_mapping" "good" {
  event_source_arn        = aws_sqs_queue.good.arn
  function_name           = aws_lambda_function.good.arn
  function_response_types = ["ReportBatchItemFailures"]
  scaling_config {
    maximum_concurrency = 2
  }
}

resource "aws_lambda_function" "bad" {
  function_name = "bad"
  role          = aws_iam_role.shared.arn
  runtime       = "nodejs22.x"
  handler       = "index.handler"
  filename      = "bad.zip"
  timeout       = 60
}

resource "aws_lambda_event_source_mapping" "bad" {
  event_source_arn = aws_sqs_queue.bad.arn
  function_name    = aws_lambda_function.bad.arn
}

resource "aws_lambda_function" "lonely" {
  function_name = "lonely"
  role          = aws_iam_role.shared.arn
  runtime       = "nodejs22.x"
  handler       = "index.handler"
  filename      = "lonely.zip"
}

# --- state machines -----------------------------------------------------------

resource "aws_sfn_state_machine" "bad" {
  name       = "bad"
  role_arn   = aws_iam_role.good.arn
  definition = jsonencode({ StartAt = "Done", States = { Done = { Type = "Succeed" } } })
}

# --- data ---------------------------------------------------------------------

resource "aws_cloudwatch_log_group" "bad" {
  name = "/bad"
}

resource "aws_kms_key" "bad" {
  description = "bad"
}

resource "aws_rds_cluster" "bad" {
  cluster_identifier = "bad"
  engine             = "aurora-postgresql"
  master_username    = "admin"
}

resource "aws_dynamodb_table" "bad" {
  name         = "bad"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "pk"
  attribute {
    name = "pk"
    type = "S"
  }
}

resource "aws_s3_bucket" "bad" {
  bucket = "bad"
}

resource "aws_ecr_repository" "bad" {
  name = "bad"
}
