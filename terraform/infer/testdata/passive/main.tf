provider "aws" {
  region = "us-east-1"
}

resource "aws_iam_role" "fn" {
  name = "fn"
}

resource "aws_lambda_function" "fn" {
  function_name = "fn"
  role          = aws_iam_role.fn.arn
}

resource "aws_sns_topic" "alerts" {
  name = "alerts"
}

# The alarm names the function in its dimensions and its description, and
# tells the topic: it watches and reports, it calls nothing.
resource "aws_cloudwatch_metric_alarm" "errors" {
  alarm_name        = "errors"
  namespace         = "AWS/Lambda"
  metric_name       = "Errors"
  dimensions        = { FunctionName = aws_lambda_function.fn.function_name }
  period            = 60
  alarm_description = "${aws_lambda_function.fn.function_name} failed"
  alarm_actions     = [aws_sns_topic.alerts.arn]
}
