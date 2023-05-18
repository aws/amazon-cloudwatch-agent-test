// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

variable "region" {
  type    = string
  default = "us-west-2"
  nullable = false
}

variable "test_dir" {
  type    = string
  default = "./test/metric_value_benchmark"
  nullable = false
}

variable "cwagent_image_repo" {
  type    = string
  default = "public.ecr.aws/cloudwatch-agent/cloudwatch-agent"
  nullable = false
}

variable "cwagent_image_tag" {
  type    = string
  default = "latest"
  nullable = false
}

variable "k8s_version" {
  type    = string
  default = "1.24"
  nullable = false
}