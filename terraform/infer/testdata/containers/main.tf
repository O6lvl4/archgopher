provider "aws" {
  region = "ap-northeast-1"
}

resource "aws_ecs_cluster" "main" {
  name = "main"
}

resource "aws_dynamodb_table" "orders" {
  name         = "orders"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "id"
}

resource "aws_ecs_task_definition" "app" {
  family                   = "app"
  requires_compatibilities = ["FARGATE"]
  cpu                      = 512
  memory                   = 1024
  runtime_platform {
    cpu_architecture        = "ARM64"
    operating_system_family = "LINUX"
  }
  container_definitions = jsonencode([{
    name        = "app"
    image       = "app:latest"
    environment = [{ name = "TABLE", value = aws_dynamodb_table.orders.name }]
  }])
}

# The service reads its task size through the task definition it names, and
# sends load neither to the task definition nor to the cluster.
resource "aws_ecs_service" "app" {
  name            = "app"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.app.arn
  desired_count   = 2
  capacity_provider_strategy {
    capacity_provider = "FARGATE"
    weight            = 1
  }
}

resource "aws_eks_cluster" "k8s" {
  name     = "k8s"
  version  = "1.35"
  role_arn = "arn:aws:iam::123456789012:role/eks"
  vpc_config {
    subnet_ids = ["subnet-1", "subnet-2"]
  }
}

resource "aws_eks_fargate_profile" "default" {
  cluster_name           = aws_eks_cluster.k8s.name
  fargate_profile_name   = "default"
  pod_execution_role_arn = "arn:aws:iam::123456789012:role/pods"
  selector {
    namespace = "default"
  }
}
