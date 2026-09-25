provider "aws" {
  region = "us-east-1"
}

resource "aws_sns_topic" "orders" {
  name = "orders"
}

resource "aws_sqs_queue" "billing" {
  name = "billing"
}

resource "aws_sns_topic_subscription" "billing" {
  topic_arn = aws_sns_topic.orders.arn
  protocol  = "sqs"
  endpoint  = aws_sqs_queue.billing.arn
}

# An HTTPS endpoint is outside the graph: the subscription is the last node.
resource "aws_sns_topic_subscription" "webhook" {
  topic_arn = aws_sns_topic.orders.arn
  protocol  = "https"
  endpoint  = "https://example.com/orders"
}

resource "aws_cloudwatch_event_bus" "app" {
  name = "app"
}

resource "aws_cloudwatch_event_rule" "orders" {
  name           = "orders"
  event_bus_name = aws_cloudwatch_event_bus.app.name
  event_pattern  = jsonencode({ source = ["app.orders"] })
}

resource "aws_cloudwatch_event_target" "orders" {
  rule           = aws_cloudwatch_event_rule.orders.name
  event_bus_name = aws_cloudwatch_event_bus.app.name
  arn            = aws_sns_topic.orders.arn
}
