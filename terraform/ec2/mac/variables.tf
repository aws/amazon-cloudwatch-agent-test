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
  default = "amzn-ec2-macos-13.*"
}

variable "ssh_key_value" {
  type    = string
  default = ""
}

variable "arc" {
  type    = string
  default = "arm64"

  validation {
    condition     = contains(["amd64", "arm64"], var.arc)
    error_message = "Valid values for arc are (amd64, arm64)."
  }
}

variable "s3_bucket" {
  type    = string
  default = ""
}

variable "test_name" {
  type    = string
  default = ""
}

variable "github_test_repo" {
  type    = string
  default = "https://github.com/aws/amazon-cloudwatch-agent-test.git"
}

variable "github_test_repo_branch" {
  type    = string
  default = "main"
}

variable "test_dir" {
  type    = string
  default = "../../../test/feature/mac" # This is really only used during tf destroy. See https://github.com/hashicorp/terraform/issues/23552
}

variable "cwa_github_sha" {
  type    = string
  default = ""
}

variable "license_manager_arn" {
  type    = string
  default = ""
}

