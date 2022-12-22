// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

variable "cwa_iam_role" {
  type    = string
  default = "cwa-e2e-iam-role"
}

variable "cwa_iam_policy" {
  type    = string
  default = "cwa-e2e-iam-policy"
}

variable "cwa_iam_instance_profile" {
  type    = string
  default = "cwa-e2e-iam-instance-profile"
}

variable "ec2_key_pair" {
  type    = string
  default = "ec2-key-pair"
}


variable "cwagent_image_repo" {
  type    = string
  default = "public.ecr.aws/cloudwatch-agent/cloudwatch-agent"
}

variable "cwagent_image_tag" {
  type    = string
  default = "latest"
}

variable "vpc_security_group" {
  type    = string
  default = "vpc_security_group"
}