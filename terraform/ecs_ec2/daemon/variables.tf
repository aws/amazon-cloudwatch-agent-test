variable "region" {
  type    = string
  default = "us-west-2"
  validation {
    condition     = can(regex("^[a-z][a-z0-9-]*$", var.region))
    error_message = "Region must contain only lowercase letters, digits, and hyphens."
  }
}

variable "ami" {
  type    = string
  default = "cloudwatch-agent-integration-test-ubuntu*"
}

variable "ec2_instance_type" {
  type    = string
  default = "t3a.xlarge"
}

variable "test_dir" {
  type    = string
  default = "./test/metric_value_benchmark"
  validation {
    condition     = can(regex("^[a-zA-Z0-9_./-]+$", var.test_dir))
    error_message = "test_dir must contain only alphanumeric characters, dots, underscores, hyphens, and forward slashes."
  }
}

variable "cwagent_image_repo" {
  type    = string
  default = "public.ecr.aws/cloudwatch-agent/cloudwatch-agent"
}

variable "cwagent_image_tag" {
  type    = string
  default = "latest"
}

variable "metadataEnabled" {
  type    = string
  default = "enabled"
  validation {
    condition     = contains(["enabled", "disabled"], var.metadataEnabled)
    error_message = "metadataEnabled must be either 'enabled' or 'disabled'."
  }
}