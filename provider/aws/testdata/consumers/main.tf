# One function consumes two queues made with for_each; its role may also send
# to them, so the IAM edge back to the queues would close a cycle with the
# event source mappings. A second function consumes one queue.
provider "aws" {
  region = "us-east-1"
}

locals {
  lanes = toset(["daily", "rewrite"])
}

resource "aws_sqs_queue" "work" {
  for_each = local.lanes
  name     = "work-${each.key}"
}

resource "aws_sqs_queue" "single" {
  name = "single"
}

resource "aws_iam_role" "worker" {
  name               = "worker"
  assume_role_policy = "{}"
}

data "aws_iam_policy_document" "worker" {
  statement {
    actions   = ["sqs:SendMessage", "sqs:ReceiveMessage", "sqs:DeleteMessage"]
    resources = [for q in aws_sqs_queue.work : q.arn]
  }
}

resource "aws_iam_role_policy" "worker" {
  role   = aws_iam_role.worker.id
  policy = data.aws_iam_policy_document.worker.json
}

resource "aws_lambda_function" "worker" {
  function_name = "worker"
  role          = aws_iam_role.worker.arn
  runtime       = "nodejs22.x"
  handler       = "index.handler"
  filename      = "worker.zip"
}

resource "aws_lambda_event_source_mapping" "worker" {
  for_each         = local.lanes
  event_source_arn = aws_sqs_queue.work[each.key].arn
  function_name    = aws_lambda_function.worker.arn
}

resource "aws_lambda_function" "solo" {
  function_name = "solo"
  role          = aws_iam_role.worker.arn
  runtime       = "nodejs22.x"
  handler       = "index.handler"
  filename      = "solo.zip"
}

resource "aws_lambda_event_source_mapping" "solo" {
  event_source_arn = aws_sqs_queue.single.arn
  function_name    = aws_lambda_function.solo.arn
}
