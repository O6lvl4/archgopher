provider "aws" {
  region = "us-east-1"
}

# Managed elsewhere: read, not owned.
data "aws_dynamodb_table" "shared" {
  name = "shared"
}

resource "aws_dynamodb_table" "own" {
  name         = "own"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "id"
}

resource "aws_iam_role" "fn" {
  name = "fn"
}

resource "aws_iam_role_policy" "fn" {
  role = aws_iam_role.fn.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["dynamodb:GetItem"]
      Resource = [data.aws_dynamodb_table.shared.arn, aws_dynamodb_table.own.arn]
    }]
  })
}

resource "aws_lambda_function" "fn" {
  function_name = "fn"
  role          = aws_iam_role.fn.arn
}
