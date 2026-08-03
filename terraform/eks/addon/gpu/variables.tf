// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

variable "region" {
  type    = string
  default = "us-west-2"
  validation {
    condition     = can(regex("^[a-z][a-z0-9-]*$", var.region))
    error_message = "Region must contain only lowercase letters, digits, and hyphens."
  }
}

variable "test_dir" {
  type    = string
  default = "./test/gpu"
  validation {
    condition     = can(regex("^[a-zA-Z0-9_./-]+$", var.test_dir))
    error_message = "test_dir must contain only alphanumeric characters, dots, underscores, hyphens, and forward slashes."
  }
}

variable "addon_name" {
  type    = string
  default = "amazon-cloudwatch-observability"
}

variable "k8s_version" {
  type    = string
  default = "1.32"
}

variable "ami_type" {
  type    = string
  default = "AL2_x86_64_GPU"
}

variable "instance_type" {
  type    = string
  default = "g4dn.xlarge"
}

variable "beta" {
  type    = bool
  default = false
}

variable "beta_endpoint" {
  type    = string
  default = "https://api.beta.us-west-2.wesley.amazonaws.com"
  validation {
    condition     = can(regex("^https://[a-zA-Z0-9._-]+\\.amazonaws\\.com(/[a-zA-Z0-9._/-]*)?$", var.beta_endpoint))
    error_message = "beta_endpoint must be a valid HTTPS amazonaws.com URL."
  }
}

variable "cwagent_image_repo" {
  type        = string
  description = "CloudWatch Agent image repository"
  default     = "public.ecr.aws/cloudwatch-agent/cloudwatch-agent"
  validation {
    condition     = can(regex("^[a-zA-Z0-9._/:@-]+$", var.cwagent_image_repo))
    error_message = "cwagent_image_repo must contain only valid container registry characters."
  }
}

variable "cwagent_image_tag" {
  type        = string
  description = "CloudWatch Agent image tag"
  default     = "latest"
  validation {
    condition     = can(regex("^[a-zA-Z0-9._-]+$", var.cwagent_image_tag))
    error_message = "cwagent_image_tag must contain only alphanumeric characters, dots, underscores, and hyphens."
  }
}