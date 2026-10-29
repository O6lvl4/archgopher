# A function module in the style of community modules: event sources come in
# as a map of objects and are made with for_each.
variable "name" { type = string }
variable "event_source_mapping" {
  type    = any
  default = {}
}

resource "aws_iam_role" "this" {
  name               = var.name
  assume_role_policy = "{}"
}

resource "aws_lambda_function" "this" {
  function_name = var.name
  role          = aws_iam_role.this.arn
  runtime       = "python3.12"
  handler       = "index.handler"
  filename      = "fn.zip"
}

resource "aws_lambda_event_source_mapping" "this" {
  for_each         = var.event_source_mapping
  event_source_arn = try(each.value.event_source_arn, null)
  function_name    = aws_lambda_function.this.arn
  batch_size       = try(each.value.batch_size, null)
}
