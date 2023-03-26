// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

variable "region" {
  type    = string
  default = "us-west-2"
}

variable "instance_type" {
  type    = string
  default = "t3a.medium"
}

variable "ami" {
  type    = string
  default = "cloudwatch-agent-integration-test-win-2022*"
}

variable "arc" {
  type    = string
  default = "amd64"
}

variable "s3_bucket" {
  type    = string
  default = ""
}

variable "ssh_key_name" {
  type    = string
  default = ""
}

variable "ssh_key_value" {
  type    = string
  default = ""
}

variable "family" {
  type    = string
  default = "linux"
}

variable "test_dir" {
  type    = string
  default = "../../../test/feature"
}

