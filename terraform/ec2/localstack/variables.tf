// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

variable "ec2_instance_type" {
  type    = string
  default = "m6g.medium"
}

variable "ssh_key_name" {
  type    = string
  default = ""
}

variable "region" {
  type    = string
  default = "us-west-2"
}

variable "ssh_key_value" {
  type    = string
  default = ""
}

variable "cwa_github_sha" {
  type    = string
  default = ""
}

variable "cwa_test_github_sha" {
  type    = string
  default = ""
}

variable "github_test_repo" {
  type    = string
  default = "https://github.com/aws/amazon-cloudwatch-agent-test.git"
}

variable "s3_bucket" {
  type    = string
  default = ""
}

variable "github_test_repo_branch" {
  default = "main"
}