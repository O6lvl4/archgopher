provider "aws" {
  region = "ap-northeast-1"
}

resource "aws_lambda_function" "diff" {
  function_name = "diff"
  memory_size   = 256
}

resource "aws_lambda_function" "copy" {
  function_name = "copy"
  memory_size   = 256
}

locals {
  # Known only after apply: the definition is read with the larger branch.
  chain = aws_lambda_function.copy.arn != ""
}

# States assembled with merge() and conditionals, the way a definition grows
# an optional tail. HCL cannot unify {Next = ...} with {} and errors on the
# conditional, which must not lose the whole definition.
resource "aws_sfn_state_machine" "assembled" {
  name     = "assembled"
  role_arn = "arn:aws:iam::123456789012:role/sfn"
  definition = jsonencode({
    StartAt = "Diff"
    States = merge({
      Diff = { Type = "Task", Resource = aws_lambda_function.diff.arn, Next = "Each" }
      Each = merge({
        Type      = "Map"
        ItemsPath = "$.items"
        ItemProcessor = {
          ProcessorConfig = { Mode = "INLINE" }
          StartAt         = "Copy"
          States          = { Copy = { Type = "Task", Resource = aws_lambda_function.copy.arn, End = true } }
        }
      }, local.chain ? { Next = "Has" } : { End = true })
      },
      local.chain ? { Has = { Type = "Choice", Choices = [{ Variable = "$.items[0]", IsPresent = true, Next = "Start" }], Default = "Done" } } : {},
      local.chain ? { Start = { Type = "Task", Resource = "arn:aws:states:::states:startExecution", Parameters = { StateMachineArn = "arn:aws:states:ap-northeast-1:123456789012:stateMachine:etl" }, End = true } } : {},
      local.chain ? { Done = { Type = "Succeed" } } : {},
    )
  })
}

# The same machine written out with the tail present.
resource "aws_sfn_state_machine" "plain" {
  name     = "plain"
  role_arn = "arn:aws:iam::123456789012:role/sfn"
  definition = jsonencode({
    StartAt = "Diff"
    States = {
      Diff = { Type = "Task", Resource = aws_lambda_function.diff.arn, Next = "Each" }
      Each = {
        Type      = "Map"
        ItemsPath = "$.items"
        ItemProcessor = {
          ProcessorConfig = { Mode = "INLINE" }
          StartAt         = "Copy"
          States          = { Copy = { Type = "Task", Resource = aws_lambda_function.copy.arn, End = true } }
        }
        Next = "Has"
      }
      Has   = { Type = "Choice", Choices = [{ Variable = "$.items[0]", IsPresent = true, Next = "Start" }], Default = "Done" }
      Start = { Type = "Task", Resource = "arn:aws:states:::states:startExecution", Parameters = { StateMachineArn = "arn:aws:states:ap-northeast-1:123456789012:stateMachine:etl" }, End = true }
      Done  = { Type = "Succeed" }
    }
  })
}
