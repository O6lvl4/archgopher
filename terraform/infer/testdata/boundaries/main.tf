provider "aws" {
  region = "us-east-1"
}

# A VPC managed here.
resource "aws_vpc" "own" {
  cidr_block = "10.0.0.0/16"
  tags       = { Name = "own" }
}

resource "aws_security_group" "fn" {
  vpc_id = aws_vpc.own.id
}

resource "aws_lambda_function" "inside" {
  function_name = "inside"
  vpc_config {
    subnet_ids         = ["subnet-1"]
    security_group_ids = [aws_security_group.fn.id]
  }
  environment {
    variables = { DB = aws_rds_cluster.db.endpoint }
  }
}

# A VPC managed elsewhere, looked up twice the same way (as modules do).
data "aws_vpc" "shared" {
  filter {
    name   = "tag:Name"
    values = ["shared"]
  }
}

data "aws_vpc" "shared_again" {
  filter {
    name   = "tag:Name"
    values = ["shared"]
  }
}

data "aws_subnets" "private" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.shared.id]
  }
}

resource "aws_db_subnet_group" "db" {
  subnet_ids = data.aws_subnets.private.ids
}

resource "aws_rds_cluster" "db" {
  engine               = "aurora-postgresql"
  db_subnet_group_name = aws_db_subnet_group.db.name
}

resource "aws_vpc_endpoint" "s3" {
  vpc_id       = data.aws_vpc.shared_again.id
  service_name = "com.amazonaws.us-east-1.s3"
}

# Calls the database but runs outside any VPC.
resource "aws_lambda_function" "outside" {
  function_name = "outside"
  environment {
    variables = { DB = aws_rds_cluster.db.endpoint }
  }
}

# Names the subnets of the tasks it starts; the state machine itself is not in them.
resource "aws_sfn_state_machine" "flow" {
  name = "flow"
  definition = jsonencode({
    StartAt = "Run"
    States = { Run = { Type = "Task", Parameters = { SecurityGroups = [aws_security_group.fn.id] }, End = true } }
  })
}
