variable "teams" {
  type = list(string)
}

module "region" {
  source = "./modules/queues"
  count  = 2
}

resource "aws_sqs_queue" "per_team" {
  count = length(var.teams)
  name  = "team-${count.index}"
}
