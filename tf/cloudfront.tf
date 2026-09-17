resource "aws_cloudfront_origin_access_control" "site" {
  name                              = "${local.cloudfront_name_prefix}-oac"
  description                       = "Private S3 origin access for ${var.domain_name}"
  signing_behavior                  = "always"
  signing_protocol                  = "sigv4"
  origin_access_control_origin_type = "s3"
}

resource "aws_cloudfront_cache_policy" "site" {
  name        = "${local.cloudfront_name_prefix}-cache-v1"
  comment     = "Honor origin cache headers"
  max_ttl     = 31536000
  min_ttl     = 0
  default_ttl = 0

  parameters_in_cache_key_and_forwarded_to_origin {
    enable_accept_encoding_brotli = true
    enable_accept_encoding_gzip   = true

    cookies_config {
      cookie_behavior = "none"
    }

    headers_config {
      header_behavior = "none"
    }

    query_strings_config {
      query_string_behavior = "none"
    }
  }
}

resource "aws_cloudfront_response_headers_policy" "site" {
  name    = "${local.cloudfront_name_prefix}-headers-v1"
  comment = "Security headers for ${var.domain_name}"

  security_headers_config {
    content_security_policy {
      content_security_policy = local.content_security_policy
      override                = true
    }

    content_type_options {
      override = true
    }

    frame_options {
      frame_option = "DENY"
      override     = true
    }

    referrer_policy {
      override        = true
      referrer_policy = "strict-origin-when-cross-origin"
    }

    strict_transport_security {
      access_control_max_age_sec = 31536000
      include_subdomains         = true
      override                   = true
      preload                    = true
    }

    xss_protection {
      mode_block = true
      override   = true
      protection = true
    }
  }

  custom_headers_config {
    items {
      header   = "Cross-Origin-Opener-Policy"
      value    = "same-origin"
      override = true
    }

    items {
      header   = "Cross-Origin-Resource-Policy"
      value    = "same-origin"
      override = true
    }

    items {
      header   = "Permissions-Policy"
      value    = "camera=(), geolocation=(), microphone=(), payment=(), usb=()"
      override = true
    }
  }
}

resource "aws_cloudfront_distribution" "site" {
  comment             = "Limanix documentation"
  price_class         = "PriceClass_100"
  default_root_object = "index.html"
  http_version        = "http2and3"
  enabled             = true
  is_ipv6_enabled     = true

  aliases = [var.domain_name]

  origin {
    domain_name              = aws_s3_bucket.site.bucket_regional_domain_name
    origin_access_control_id = aws_cloudfront_origin_access_control.site.id
    origin_id                = local.cloudfront_origin_id
  }

  default_cache_behavior {
    response_headers_policy_id = aws_cloudfront_response_headers_policy.site.id
    cache_policy_id            = aws_cloudfront_cache_policy.site.id
    target_origin_id           = local.cloudfront_origin_id
    allowed_methods            = ["GET", "HEAD"]
    cached_methods             = ["GET", "HEAD"]
    viewer_protocol_policy     = "redirect-to-https"
    compress                   = true

    function_association {
      event_type   = "viewer-request"
      function_arn = aws_cloudfront_function.docs_uri_rewrite.arn
    }
  }

  custom_error_response {
    error_caching_min_ttl = 30
    error_code            = 403
    response_code         = 404
    response_page_path    = "/404.html"
  }

  custom_error_response {
    error_caching_min_ttl = 30
    error_code            = 404
    response_code         = 404
    response_page_path    = "/404.html"
  }

  restrictions {
    geo_restriction {
      restriction_type = "none"
    }
  }

  viewer_certificate {
    acm_certificate_arn      = var.acm_certificate_arn
    minimum_protocol_version = "TLSv1.2_2021"
    ssl_support_method       = "sni-only"
  }
}

resource "aws_cloudfront_function" "docs_uri_rewrite" {
  name    = "${local.cloudfront_name_prefix}-uri-rewrite"
  runtime = "cloudfront-js-2.0"
  comment = "Resolve limanix docs directory URLs to their S3 objects"
  publish = true
  code    = file("${path.module}/functions/limanix-uri-rewrite.js")
}
