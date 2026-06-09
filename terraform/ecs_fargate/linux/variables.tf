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
  default = "./test/ecs/service_discovery"
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