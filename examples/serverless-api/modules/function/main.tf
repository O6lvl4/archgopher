variable "name" {
  type = string
}

variable "memory_size" {
  type    = number
  default = 128
}

variable "architecture" {
  type    = string
  default = "x86_64"
}

variable "environment" {
  type    = map(string)
  default = {}
}

variable "statements" {
  type = list(object({
    actions   = list(string)
    resources = list(string)
  }))
  default = []
}

resource "aws_iam_role" "this" {
  name = var.name
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Action    = "sts:AssumeRole"
      Principal = { Service = "lambda.amazonaws.com" }
    }]
  })
}

data "aws_iam_policy_document" "this" {
  dynamic "statement" {
    for_each = var.statements
    content {
      actions   = statement.value.actions
      resources = statement.value.resources
    }
  }
}

resource "aws_iam_role_policy" "this" {
  role   = aws_iam_role.this.id
  policy = data.aws_iam_policy_document.this.json
}

resource "aws_lambda_function" "this" {
  function_name = var.name
  role          = aws_iam_role.this.arn
  handler       = "index.handler"
  runtime       = "nodejs22.x"
  filename      = "${path.module}/dist.zip"
  memory_size   = var.memory_size
  architectures = [var.architecture]

  environment {
    variables = var.environment
  }
}

output "arn" {
  value = aws_lambda_function.this.arn
}

output "invoke_arn" {
  value = aws_lambda_function.this.invoke_arn
}

output "function_name" {
  value = aws_lambda_function.this.function_name
}
