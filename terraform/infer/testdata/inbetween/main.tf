provider "aws" {
  region = "us-east-1"
}

resource "aws_sqs_queue" "orders" {
  name = "orders"
}

resource "aws_lambda_function" "enrich" {
  function_name = "enrich"
}

resource "aws_lambda_function" "handler" {
  function_name = "handler"
}

# The pipe polls the queue and calls the enrichment and the target.
resource "aws_pipes_pipe" "orders" {
  name       = "orders"
  source     = aws_sqs_queue.orders.arn
  enrichment = aws_lambda_function.enrich.arn
  target     = aws_lambda_function.handler.arn
}

resource "aws_kinesis_stream" "clicks" {
  name        = "clicks"
  shard_count = 1
}

resource "aws_s3_bucket" "archive" {
  bucket = "archive"
}

# The delivery stream reads the Kinesis stream and writes to the bucket.
resource "aws_kinesis_firehose_delivery_stream" "archive" {
  name        = "archive"
  destination = "extended_s3"
  kinesis_source_configuration {
    kinesis_stream_arn = aws_kinesis_stream.clicks.arn
  }
  extended_s3_configuration {
    bucket_arn = aws_s3_bucket.archive.arn
  }
}
