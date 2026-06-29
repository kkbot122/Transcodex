locals {
  s3_origin_id  = "${local.name}-s3"
  api_origin_id = "${local.name}-alb"
}

resource "aws_cloudfront_origin_access_control" "storage" {
  name                              = "${local.name}-storage"
  description                       = "Transcodex private S3 access"
  origin_access_control_origin_type = "s3"
  signing_behavior                  = "always"
  signing_protocol                  = "sigv4"
}

resource "aws_cloudfront_distribution" "main" {
  enabled             = true
  is_ipv6_enabled     = true
  comment             = "${local.name} CDN"
  default_root_object = "demo/index.html"
  aliases             = var.cloudfront_aliases
  price_class         = var.cloudfront_price_class

  origin {
    origin_id                = local.s3_origin_id
    domain_name              = aws_s3_bucket.storage.bucket_regional_domain_name
    origin_access_control_id = aws_cloudfront_origin_access_control.storage.id
  }

  origin {
    origin_id   = local.api_origin_id
    domain_name = aws_lb.app.dns_name

    custom_origin_config {
      http_port              = 80
      https_port             = 443
      origin_protocol_policy = "http-only"
      origin_ssl_protocols   = ["TLSv1.2"]
    }
  }

  default_cache_behavior {
    target_origin_id       = local.s3_origin_id
    viewer_protocol_policy = "redirect-to-https"
    allowed_methods        = ["GET", "HEAD", "OPTIONS"]
    cached_methods         = ["GET", "HEAD"]
    compress               = true
    min_ttl                = 0
    default_ttl            = 300
    max_ttl                = 3600

    forwarded_values {
      query_string = false

      cookies {
        forward = "none"
      }
    }
  }

  ordered_cache_behavior {
    path_pattern           = "/outputs/*"
    target_origin_id       = local.s3_origin_id
    viewer_protocol_policy = "redirect-to-https"
    allowed_methods        = ["GET", "HEAD", "OPTIONS"]
    cached_methods         = ["GET", "HEAD"]
    compress               = true
    min_ttl                = 0
    default_ttl            = 86400
    max_ttl                = 31536000

    forwarded_values {
      query_string = false

      cookies {
        forward = "none"
      }
    }
  }

  dynamic "ordered_cache_behavior" {
    for_each = toset(["/uploads", "/jobs/*", "/internal/*", "/healthz"])

    content {
      path_pattern           = ordered_cache_behavior.value
      target_origin_id       = local.api_origin_id
      viewer_protocol_policy = "redirect-to-https"
      allowed_methods        = ["DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT"]
      cached_methods         = ["GET", "HEAD"]
      compress               = true
      min_ttl                = 0
      default_ttl            = 0
      max_ttl                = 0

      forwarded_values {
        query_string = true
        headers      = ["Authorization", "Content-Type", "Origin"]

        cookies {
          forward = "all"
        }
      }
    }
  }

  restrictions {
    geo_restriction {
      restriction_type = "none"
    }
  }

  viewer_certificate {
    cloudfront_default_certificate = var.certificate_arn == "" ? true : null
    acm_certificate_arn            = var.certificate_arn == "" ? null : var.certificate_arn
    ssl_support_method             = var.certificate_arn == "" ? null : "sni-only"
    minimum_protocol_version       = var.certificate_arn == "" ? null : "TLSv1.2_2021"
  }
}
