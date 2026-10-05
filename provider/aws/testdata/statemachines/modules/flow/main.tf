variable "function" { type = string }

resource "aws_sfn_state_machine" "this" {
  name       = "flow"
  role_arn   = "arn:aws:iam::123456789012:role/sfn"
  definition = templatefile("${path.module}/flow.asl.json", { function = var.function })
}
