locals {
  cloudfront_name_prefix = "mr-chelyshkin-limanix-${replace(var.domain_name, ".", "-")}"
  cloudfront_origin_id   = "s3-${var.site_bucket_name}"

  content_security_policy = join("; ", [
    "default-src 'self'",
    "base-uri 'self'",
    "connect-src 'self'",
    "font-src 'self'",
    "form-action 'self'",
    "frame-ancestors 'none'",
    "img-src 'self' data:",
    "media-src 'self'",
    "object-src 'none'",
    "script-src 'self' 'unsafe-inline'",
    "script-src-attr 'none'",
    "style-src 'self' 'unsafe-inline'",
  ])

  tags = {
    ManagedBy  = "Terraform"
    Project    = "limanix-docs"
    Repository = "mr-chelyshkin/limanix"
  }
}
