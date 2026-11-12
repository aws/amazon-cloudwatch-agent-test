// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

variable "region" {
  type    = string
  default = "us-west-2"
}

variable "test_dir" {
  type    = string
  default = "./test/otel/performance"
}

variable "cwagent_image_repo" {
  type    = string
  default = "public.ecr.aws/cloudwatch-agent/cloudwatch-agent"
}

variable "cwagent_image_tag" {
  # The default only keeps `terraform destroy` (run without -var flags) from waiting on an input
  # prompt until the step times out. Every apply passes the real tag; TestRegressionCheck refuses
  # "latest", so the default can never become a regression baseline key.
  type    = string
  default = "latest"
  validation {
    condition     = length(var.cwagent_image_tag) > 0
    error_message = "cwagent_image_tag must be set; it is used as the regression baseline commit key."
  }
}

variable "helm_chart_branch" {
  type    = string
  default = "main"
}

variable "k8s_version" {
  type    = string
  default = "1.35"
}

variable "ami_type" {
  type    = string
  default = "AL2023_x86_64_STANDARD"
}

variable "instance_type" {
  type    = string
  default = "t3.medium"
}
