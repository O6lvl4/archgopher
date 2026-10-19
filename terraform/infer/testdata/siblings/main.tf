provider "aws" {
  region = "ap-northeast-1"
}

resource "aws_rds_cluster" "db" {
  engine = "aurora-postgresql"
  serverlessv2_scaling_configuration {
    min_capacity = 0
    max_capacity = 4
  }
}

# engine names the cluster's. The cluster sets it in the configuration, so it
# is known before apply; cluster_identifier names the id, which is not.
resource "aws_rds_cluster_instance" "writer" {
  cluster_identifier = aws_rds_cluster.db.id
  engine             = aws_rds_cluster.db.engine
  instance_class     = "db.serverless"
}

# A count and an attribute that read a sibling's arguments, one of them
# inside a nested block, are known too.
resource "aws_cloudwatch_log_group" "postgres" {
  count             = aws_rds_cluster.db.engine == "aurora-postgresql" ? 1 : 0
  name              = "postgres"
  retention_in_days = aws_rds_cluster.db.serverlessv2_scaling_configuration[0].max_capacity * 7
}
