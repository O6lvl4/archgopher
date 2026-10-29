provider "aws" {
  region = "us-east-1"
}

resource "aws_sqs_queue" "jobs" {
  name = "jobs"
}

resource "aws_sqs_queue" "failures" {
  name = "failures"
}

resource "aws_kinesis_stream" "events" {
  name        = "events"
  shard_count = 1
}

module "worker" {
  source = "./modules/fn"
  name   = "worker"
  event_source_mapping = {
    jobs   = { event_source_arn = aws_sqs_queue.jobs.arn, batch_size = 5 }
    events = { event_source_arn = aws_kinesis_stream.events.arn, destination_arn_on_failure = aws_sqs_queue.failures.arn }
  }
}

resource "aws_rds_cluster" "db" {
  cluster_identifier = "db"
  engine             = "aurora-postgresql"
  master_username    = "admin"
}

resource "aws_secretsmanager_secret" "db" {
  name = "db"
}

resource "aws_secretsmanager_secret_version" "db" {
  secret_id     = aws_secretsmanager_secret.db.id
  secret_string = jsonencode({ host = aws_rds_cluster.db.endpoint, port = 5432 })
}

resource "aws_ssm_parameter" "db_host" {
  name  = "/db/host"
  type  = "String"
  value = aws_rds_cluster.db.endpoint
}

resource "aws_iam_role" "fn" {
  name               = "fn"
  assume_role_policy = "{}"
}

resource "aws_iam_role_policy" "fn" {
  role = aws_iam_role.fn.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      { Effect = "Allow", Action = ["secretsmanager:GetSecretValue"], Resource = aws_secretsmanager_secret.db.arn },
      { Effect = "Allow", Action = ["ssm:GetParameter"], Resource = aws_ssm_parameter.db_host.arn },
    ]
  })
}

# Reads the connection through a secret.
resource "aws_lambda_function" "via_secret" {
  function_name = "via-secret"
  role          = aws_iam_role.fn.arn
  runtime       = "python3.12"
  handler       = "index.handler"
  filename      = "fn.zip"
  environment {
    variables = { DB_SECRET_ARN = aws_secretsmanager_secret.db.arn }
  }
}

# Reads the connection through a parameter.
resource "aws_lambda_function" "via_parameter" {
  function_name = "via-parameter"
  role          = aws_iam_role.fn.arn
  runtime       = "python3.12"
  handler       = "index.handler"
  filename      = "fn.zip"
  environment {
    variables = { DB_HOST_PARAMETER = aws_ssm_parameter.db_host.name }
  }
}
