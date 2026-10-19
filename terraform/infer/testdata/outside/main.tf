provider "aws" {
  region = "ap-northeast-1"
}

variable "model_id" {
  default = "anthropic.claude-haiku-4-5-20251001-v1:0"
}

# A guardrail another account owns; the caller pays for applying it.
variable "guardrail_arn" {
  default = "arn:aws:bedrock:ap-northeast-1:111122223333:guardrail/abc123"
}

# Permissions on things this configuration does not own: a model by two
# ARNs, the guardrail, another team's bucket. Listing models and writing
# logs carry no load and make nothing.
resource "aws_iam_role" "agent" {
  name = "agent"
  inline_policy {
    name = "outside"
    policy = jsonencode({
      Statement = [
        {
          Effect = "Allow"
          Action = ["bedrock:InvokeModel", "bedrock:InvokeModelWithResponseStream"]
          Resource = [
            "arn:aws:bedrock:ap-northeast-1::foundation-model/${var.model_id}",
            "arn:aws:bedrock:ap-northeast-1:*:inference-profile/jp.${var.model_id}",
          ]
        },
        { Effect = "Allow", Action = "bedrock:ApplyGuardrail", Resource = var.guardrail_arn },
        { Effect = "Allow", Action = "s3:GetObject", Resource = "arn:aws:s3:::shared-datasets/*" },
        { Effect = "Allow", Action = "bedrock:ListFoundationModels", Resource = "*" },
        { Effect = "Allow", Action = ["logs:CreateLogStream", "logs:PutLogEvents"], Resource = "*" },
      ]
    })
  }
}

resource "aws_lambda_function" "agent" {
  function_name = "agent"
  role          = aws_iam_role.agent.arn
  memory_size   = 512
}

# Any model: the ARN is not written, but the action can only mean a model.
resource "aws_iam_role" "chat" {
  name = "chat"
  inline_policy {
    name = "any-model"
    policy = jsonencode({
      Statement = [
        { Effect = "Allow", Action = "bedrock:InvokeModel", Resource = "*" },
        { Effect = "Allow", Action = "bedrock:InvokeModelWithResponseStream", Resource = "arn:aws:bedrock:*::foundation-model/*" },
      ]
    })
  }
}

resource "aws_lambda_function" "chat" {
  function_name = "chat"
  role          = aws_iam_role.chat.arn
  memory_size   = 512
}
