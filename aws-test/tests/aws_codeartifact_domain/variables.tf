
variable "resource_name" {
  type        = string
  default     = "turbot-test-20200125"
  description = "Name of the resource used throughout the test."
}

variable "aws_profile" {
  type        = string
  default     = "default"
  description = "AWS credentials profile used for the test. Default is to use the default profile."
}

variable "aws_region" {
  type        = string
  default     = "us-east-1"
  description = "AWS region used for the test. Does not work with default region in config, so must be defined here."
}

variable "aws_region_alternate" {
  type        = string
  default     = "us-east-2"
  description = "Alternate AWS region used for tests that require two regions."
}

provider "aws" {
  profile = var.aws_profile
  region  = var.aws_region
}

provider "aws" {
  alias   = "alternate"
  profile = var.aws_profile
  region  = var.aws_region_alternate
}

data "aws_partition" "current" {}
data "aws_caller_identity" "current" {}
data "aws_region" "primary" {}
data "aws_region" "alternate" {
  provider = aws.alternate
}

data "null_data_source" "resource" {
  inputs = {
    scope = "arn:${data.aws_partition.current.partition}:::${data.aws_caller_identity.current.account_id}"
  }
}

resource "aws_codeartifact_domain" "named_test_resource" {
  domain = var.resource_name
  tags = {
    name = var.resource_name
  }
}

output "resource_aka" {
  value = aws_codeartifact_domain.named_test_resource.arn
}

output "owner" {
  value = aws_codeartifact_domain.named_test_resource.owner
}

output "repository_count" {
  value = aws_codeartifact_domain.named_test_resource.repository_count
}

output "asset_size_bytes" {
  value = aws_codeartifact_domain.named_test_resource.asset_size_bytes
}

output "tags_src" {
  value = aws_codeartifact_domain.named_test_resource.tags_all
}

output "resource_name" {
  value = var.resource_name
}
