provider "aws" {
  region = "ap-northeast-1"
}

# Users reach the accelerator; its listener stands for it, and the endpoint
# group that names the listener is called through it and calls the load balancer.
resource "aws_globalaccelerator_accelerator" "edge" {
  name = "edge"
}

resource "aws_globalaccelerator_listener" "https" {
  accelerator_arn = aws_globalaccelerator_accelerator.edge.id
  protocol        = "TCP"
  port_range {
    from_port = 443
    to_port   = 443
  }
}

resource "aws_globalaccelerator_endpoint_group" "tokyo" {
  listener_arn = aws_globalaccelerator_listener.https.id
  endpoint_configuration {
    endpoint_id = aws_lb.web.arn
  }
}

resource "aws_lb" "web" {
  name = "web"
  subnet_mapping {
    subnet_id     = "subnet-a"
    allocation_id = aws_eip.a.id
  }
  subnet_mapping {
    subnet_id     = "subnet-c"
    allocation_id = aws_eip.c.id
  }
}

resource "aws_eip" "a" {}
resource "aws_eip" "c" {}

# The record points at the load balancer and the private certificate is
# validated through the zone: answers and names, not calls.
resource "aws_route53_zone" "main" {
  name = "example.com"
}

resource "aws_route53_record" "www" {
  zone_id = aws_route53_zone.main.zone_id
  name    = "www"
  type    = "A"
  alias {
    name                   = aws_lb.web.dns_name
    zone_id                = aws_lb.web.zone_id
    evaluate_target_health = true
  }
}

resource "aws_acmpca_certificate_authority" "internal" {
  certificate_authority_configuration {
    key_algorithm     = "RSA_4096"
    signing_algorithm = "SHA512WITHRSA"
    subject {
      common_name = "internal"
    }
  }
}

resource "aws_acm_certificate" "internal" {
  domain_name               = "internal.example.com"
  certificate_authority_arn = aws_acmpca_certificate_authority.internal.arn
}

resource "aws_route53_resolver_endpoint" "out" {
  direction          = "OUTBOUND"
  security_group_ids = ["sg-1"]
  ip_address {
    subnet_id = "subnet-a"
  }
  ip_address {
    subnet_id = "subnet-c"
  }
  ip_address {
    subnet_id = "subnet-d"
  }
}

# The stage is where requests arrive; it calls its API.
resource "aws_api_gateway_rest_api" "api" {
  name = "api"
}

resource "aws_api_gateway_deployment" "v1" {
  rest_api_id = aws_api_gateway_rest_api.api.id
}

resource "aws_api_gateway_stage" "prod" {
  rest_api_id   = aws_api_gateway_rest_api.api.id
  deployment_id = aws_api_gateway_deployment.v1.id
  stage_name    = "prod"
}
