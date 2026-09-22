variable "aws_region" {
  description = "AWS region for the documentation bucket."
  type        = string
  nullable    = false
}

variable "domain_name" {
  description = "Documentation hostname."
  type        = string
  nullable    = false
}

variable "site_bucket_name" {
  description = "S3 bucket name for the static contents."
  type        = string
  nullable    = false
}

variable "acm_certificate_arn" {
  description = "ARN of an issued ACM certificate covering domain_name."
  type        = string
  nullable    = false
}
