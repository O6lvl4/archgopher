provider "aws" { region = "us-east-1" }

resource "aws_lambda_function" "work" {
  function_name = "work"
  role          = "arn:aws:iam::123456789012:role/work"
  handler       = "index.handler"
  runtime       = "nodejs20.x"
  filename      = "work.zip"
}

# jsonencode with an ARN known only after apply.
resource "aws_sfn_state_machine" "inline" {
  name     = "inline"
  role_arn = "arn:aws:iam::123456789012:role/sfn"
  definition = jsonencode({
    StartAt = "Work"
    States = {
      Work = { Type = "Task", Resource = aws_lambda_function.work.arn, Next = "Done" }
      Done = { Type = "Succeed" }
    }
  })
}

# A heredoc interpolating the ARN.
resource "aws_sfn_state_machine" "heredoc" {
  name       = "heredoc"
  role_arn   = "arn:aws:iam::123456789012:role/sfn"
  definition = <<EOT
{"StartAt":"Work","States":{"Work":{"Type":"Task","Resource":"${aws_lambda_function.work.arn}","Next":"Tidy"},"Tidy":{"Type":"Pass","Next":"Done"},"Done":{"Type":"Succeed"}}}
EOT
}

# A file relative to the working directory, through a local.
locals {
  polling = file("polling.asl.json")
}

resource "aws_sfn_state_machine" "polling" {
  name       = "polling"
  role_arn   = "arn:aws:iam::123456789012:role/sfn"
  definition = local.polling
}

# A templatefile in a module, found through path.module.
module "flow" {
  source   = "./modules/flow"
  function = aws_lambda_function.work.arn
}

# Express workflows bill requests and duration, not transitions.
resource "aws_sfn_state_machine" "express" {
  name       = "express"
  role_arn   = "arn:aws:iam::123456789012:role/sfn"
  type       = "EXPRESS"
  definition = local.polling
}
