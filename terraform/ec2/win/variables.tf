// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

variable "region" {
  type    = string
  default = "us-west-2"
}

variable "ec2_instance_type" {
  type    = string
  default = "t3a.medium"
}

variable "ami" {
  type    = string
  default = "Windows_Server-2022-English-Full-Base*"
}

variable "cwa_github_sha" {
  type    = string
  default = "219753e3d0dac95b65ff29834d56b8ffa94cec8b"
}

variable "github_test_repo_branch" {
  type    = string
  default = "main"
}
variable "github_test_repo" {
  type    = string
  default = "https://github.com/aws/amazon-cloudwatch-agent-test.git"
}

variable "ssh_key_name" {
  type    = string
  default = ""
}

variable "ssh_key_value" {
  type    = string
  default = ""
}

variable "s3_bucket" {
  type    = string
  default = ""
}

variable "test_dir" {
  type    = string
  default = "../../../test/nvidia_gpu"
}
