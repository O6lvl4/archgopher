provider "aws" {
  region = "us-east-1"
}

locals {
  tables = {
    orders = "orders"
    audit  = "audit"
  }
}

resource "aws_dynamodb_table" "t" {
  for_each     = local.tables
  name         = each.value
  billing_mode = "PAY_PER_REQUEST"
}

resource "aws_s3_bucket" "logs" {
  bucket = replace(upper("logs-${local.tables.audit}"), "/-+/", "_")
}

resource "aws_iam_role" "worker" {
  name = "worker"
  inline_policy {
    name = "inline"
    policy = jsonencode({
      Statement = [
        { Effect = "Allow", Action = ["s3:GetObject"], Resource = "${aws_s3_bucket.logs.arn}/*" },
        { Effect = "Deny", Action = ["s3:PutObject"], Resource = "${aws_s3_bucket.logs.arn}/*" },
      ]
    })
  }
}

resource "aws_iam_policy" "tables" {
  policy = jsonencode({
    Statement = [{ Effect = "Allow", Action = "dynamodb:Query", Resource = aws_dynamodb_table.t["orders"].arn }]
  })
}

resource "aws_iam_role_policy_attachment" "tables" {
  role       = aws_iam_role.worker.name
  policy_arn = aws_iam_policy.tables.arn
}

resource "aws_lambda_function" "worker" {
  function_name = "worker"
  role          = aws_iam_role.worker.arn
  memory_size   = 2048
}

resource "aws_lambda_function_url" "worker" {
  function_name = aws_lambda_function.worker.function_name
}

resource "aws_scheduler_schedule" "nightly" {
  schedule_expression = "cron(30 2 * * ? *)"
  target {
    arn      = aws_lambda_function.worker.arn
    role_arn = aws_iam_role.worker.arn
  }
}
