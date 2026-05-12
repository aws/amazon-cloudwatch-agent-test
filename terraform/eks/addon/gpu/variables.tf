// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

variable "region" {
  type    = string
  default = "us-west-2"
}

variable "test_dir" {
  type    = string
  default = "./test/gpu"
}

variable "addon_name" {
  type    = string
  default = "amazon-cloudwatch-observability"
}

variable "k8s_version" {
  type    = string
  default = "1.34"
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
}

variable "cwagent_image_repo" {
  type        = string
  description = "CloudWatch Agent image repository"
  default     = "public.ecr.aws/cloudwatch-agent/cloudwatch-agent"
}

variable "cwagent_image_tag" {
  type        = string
  description = "CloudWatch Agent image tag"
  default     = "latest"
}