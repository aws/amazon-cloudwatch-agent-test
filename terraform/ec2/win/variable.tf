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
  default = "cloudwatch-agent-integration-test-win-2022*"
}

variable "arc" {
  type    = string
  default = "amd64"

  validation {
    condition     = contains(["amd64", "arm64"], var.arc)
    error_message = "Valid values for arc are (amd64, arm64)."
  }
}

variable "cwa_github_sha" {
  type    = string
  default = ""
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

variable "test_name" {
  type    = string
  default = ""
}

variable "test_dir" {
  type    = string
  default = "../../../test/feature/windows"
}

variable "use_ssm" {
  type    = bool
  default = false
}
