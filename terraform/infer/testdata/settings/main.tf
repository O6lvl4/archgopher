provider "aws" {
  region = "ap-northeast-1"
}

resource "aws_ec2_host" "h" {
  instance_family   = "m5"
  availability_zone = "ap-northeast-1a"
}

# host_id is known only after apply; the instance reads that it is set.
resource "aws_instance" "on_host" {
  instance_type = "m5.large"
  host_id       = aws_ec2_host.h.id
}

resource "aws_instance" "literal" {
  instance_type = "m5.large"
  host_id       = "h-0123456789abcdef0"
}

resource "aws_elastic_beanstalk_environment" "env" {
  name        = "env"
  application = "app"

  setting {
    namespace = "aws:autoscaling:asg"
    name      = "MinSize"
    value     = "2"
  }
  setting {
    namespace = "aws:autoscaling:launchconfiguration"
    name      = "InstanceType"
    value     = "t3.small"
  }
  setting {
    namespace = "aws:elasticbeanstalk:cloudwatch:logs"
    name      = "StreamLogs"
    value     = "true"
  }
}
