// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

variable "region" {
  type    = string
  default = "us-west-2"
}

variable "test_dir" {
  type    = string
  default = "../../../../test/gpu"
}

variable "addon_name" {
  type    = string
  default = "amazon-cloudwatch-observability"
}

variable "k8s_version" {
  type    = string
  default = "1.31"
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
  default = true
}

variable "beta_endpoint" {
  type    = string
  default = "https://api.beta.us-west-2.wesley.amazonaws.com"
}