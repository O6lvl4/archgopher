locals {
  servers = { web = "t3.micro", batch = "m5.large", web2 = "t3.micro" }
}

# Two sizes: one node per size, the two t3.micro servers together.
resource "aws_instance" "app" {
  for_each      = local.servers
  instance_type = each.value
  tags          = { Name = each.key }
}

# Alike but for their names: one node.
resource "aws_sqs_queue" "work" {
  count = 3
  name  = "work-${count.index}"
}
