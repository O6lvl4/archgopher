terraform {
  required_providers {
    aws = { source = "hashicorp/aws" }
  }
}

provider "aws" {
  region = var.region
}

variable "region" {
  type    = string
  default = "ap-northeast-1"
}

variable "env" {
  type = string
}

variable "enable_cleanup" {
  type    = bool
  default = true
}

locals {
  prefix = "notes-${var.env}"
}

# --- front door -----------------------------------------------------------------

resource "aws_cloudfront_distribution" "web" {
  enabled = true

  origin {
    domain_name = aws_s3_bucket.assets.bucket_regional_domain_name
    origin_id   = "assets"
  }

  origin {
    domain_name = "${aws_api_gateway_rest_api.api.id}.execute-api.${var.region}.amazonaws.com"
    origin_id   = "api"
  }

  default_cache_behavior {
    target_origin_id       = "assets"
    viewer_protocol_policy = "redirect-to-https"
    allowed_methods        = ["GET", "HEAD"]
    cached_methods         = ["GET", "HEAD"]
  }

  restrictions {
    geo_restriction {
      restriction_type = "none"
    }
  }

  viewer_certificate {
    cloudfront_default_certificate = true
  }
}

resource "aws_s3_bucket" "assets" {
  bucket = "${local.prefix}-assets"
}

resource "aws_api_gateway_rest_api" "api" {
  name = "${local.prefix}-api"
}

resource "aws_api_gateway_integration" "notes" {
  rest_api_id             = aws_api_gateway_rest_api.api.id
  resource_id             = aws_api_gateway_rest_api.api.root_resource_id
  http_method             = "ANY"
  type                    = "AWS_PROXY"
  integration_http_method = "POST"
  uri                     = module.api_handler.invoke_arn
}

# --- data -----------------------------------------------------------------------

resource "aws_dynamodb_table" "notes" {
  name         = "${local.prefix}-notes"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "id"

  attribute {
    name = "id"
    type = "S"
  }
}

resource "aws_s3_bucket" "exports" {
  bucket = "${local.prefix}-exports"
}

resource "aws_sqs_queue" "exports" {
  name = "${local.prefix}-exports"
}

# --- functions ------------------------------------------------------------------

module "api_handler" {
  source       = "./modules/function"
  name         = "${local.prefix}-api"
  memory_size  = 512
  architecture = "arm64"
  environment = {
    TABLE_NAME = aws_dynamodb_table.notes.name
    QUEUE_URL  = aws_sqs_queue.exports.url
  }
  statements = [
    {
      actions   = ["dynamodb:GetItem", "dynamodb:Query", "dynamodb:PutItem"]
      resources = [aws_dynamodb_table.notes.arn]
    },
    {
      actions   = ["sqs:SendMessage"]
      resources = [aws_sqs_queue.exports.arn]
    },
  ]
}

module "exporter" {
  source      = "./modules/function"
  name        = "${local.prefix}-exporter"
  memory_size = 3008
  environment = {
    BUCKET = aws_s3_bucket.exports.bucket
  }
  statements = [
    {
      actions   = ["s3:PutObject"]
      resources = ["${aws_s3_bucket.exports.arn}/*"]
    },
    {
      actions   = ["sqs:ReceiveMessage", "sqs:DeleteMessage", "sqs:GetQueueAttributes"]
      resources = [aws_sqs_queue.exports.arn]
    },
  ]
}

resource "aws_lambda_event_source_mapping" "exports" {
  event_source_arn = aws_sqs_queue.exports.arn
  function_name    = module.exporter.function_name
  batch_size       = 10
}

module "cleanup" {
  count       = var.enable_cleanup ? 1 : 0
  source      = "./modules/function"
  name        = "${local.prefix}-cleanup"
  memory_size = 256
  environment = {
    TABLE_NAME = aws_dynamodb_table.notes.name
  }
  statements = [
    {
      actions   = ["dynamodb:Scan", "dynamodb:DeleteItem"]
      resources = [aws_dynamodb_table.notes.arn]
    },
  ]
}

resource "aws_cloudwatch_event_rule" "cleanup" {
  count               = var.enable_cleanup ? 1 : 0
  name                = "${local.prefix}-cleanup"
  schedule_expression = "rate(1 hour)"
}

resource "aws_cloudwatch_event_target" "cleanup" {
  count = var.enable_cleanup ? 1 : 0
  rule  = aws_cloudwatch_event_rule.cleanup[0].name
  arn   = module.cleanup[0].arn
}
