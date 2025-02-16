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

variable "is_selinux_test" {
  type    = bool
  default = false
}

variable "ssh_key_name" {
  type    = string
  default = ""
}

variable "ami" {
  type    = string
  default = "cloudwatch-agent-integration-test-ubuntu*"
}

variable "ssh_key_value" {
  type    = string
  default = ""
}

variable "user" {
  type    = string
  default = ""
}

variable "install_agent" {
  description = "go run ./install/install_agent.go deb or go run ./install/install_agent.go rpm"
  type        = string
  default     = "go run ./install/install_agent.go rpm"
}

variable "ca_cert_path" {
  type    = string
  default = ""
}

variable "arc" {
  type    = string
  default = "amd64"

  validation {
    condition     = contains(["amd64", "arm64"], var.arc)
    error_message = "Valid values for arc are (amd64, arm64)."
  }
}

variable "binary_name" {
  type    = string
  default = ""
}

variable "local_stack_host_name" {
  type    = string
  default = "localhost.localstack.cloud"
}

variable "s3_bucket" {
  type    = string
  default = ""
}

variable "test_name" {
  type    = string
  default = ""
}

variable "selinux_branch" {
  type    = bool
  default = false
}

variable "test_dir" {
  type    = string
  default = ""
}

variable "cwa_github_sha" {
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

variable "is_canary" {
  type    = bool
  default = false
}

variable "excluded_tests" {
  type    = string
  default = ""
}

variable "plugin_tests" {
  type    = string
  default = ""
}

variable "agent_start" {
  description = "default command should be for ec2 with linux"
  type        = string
  default     = "sudo /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl -a fetch-config -m ec2 -s -c "
}