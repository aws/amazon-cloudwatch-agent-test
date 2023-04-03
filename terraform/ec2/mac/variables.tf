// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

variable "region" {
  type    = string
  default = "us-west-2"
}

variable "ec2_instance_type" {
  type    = string
  default = "mac2.metal"
}

variable "ssh_key_name" {
  type    = string
  default = ""
}

variable "ami" {
  type    = string
  default = "amzn-ec2-macos-13.*-arm64"
}

variable "ssh_key_value" {
  type    = string
  default = ""
}

variable "user" {
  type    = string
  default = "ec2-user"
}

variable "arc" {
  type    = string
  default = "arm64"
}

variable "s3_bucket" {
  type    = string
  default = "cloudwatch-agent-integration-bucket"
}

variable "test_name" {
  type    = string
  default = ""
}

variable "test_dir" {
  type    = string
  default = ""
}

variable "cwa_github_sha" {
  type    = string
  default = "007192508454371edb98801d62c434f2bec2f94f"
}

variable "github_test_repo" {
  type    = string
  default = "https://github.com/aws/amazon-cloudwatch-agent-test.git"
}

variable "github_test_repo_branch" {
  type    = string
  default = "main"
}
