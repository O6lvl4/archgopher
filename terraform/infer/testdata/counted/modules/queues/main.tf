resource "aws_sqs_queue" "work" {
  count = 3
  name  = "work-${count.index}"
}
