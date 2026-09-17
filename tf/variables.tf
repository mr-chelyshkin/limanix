variable "aws_region" {
  description = "AWS region for the documentation bucket."
  type        = string
  nullable    = false
}

variable "domain_name" {
  description = "Documentation hostname, without a scheme or path. DNS is managed separately."
  type        = string
  nullable    = false
}

variable "site_bucket_name" {
  description = "Globally unique S3 bucket name for the contents of build/docs."
  type        = string
  nullable    = false
}

variable "acm_certificate_arn" {
  description = "ARN of an issued ACM certificate in us-east-1 covering domain_name."
  type        = string
  nullable    = false
}
