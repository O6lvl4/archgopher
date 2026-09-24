resource "aws_cloudfront_distribution" "web" {
  enabled = true
}

resource "aws_cognito_user_pool" "users" {
  admin_create_user_config {
    invite_message_template {
      email_message = "Sign in at https://${aws_cloudfront_distribution.web.domain_name} with {username} / {####}"
    }
  }
}
