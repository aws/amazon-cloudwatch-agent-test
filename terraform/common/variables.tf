// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT


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

variable "prefix_list_id" {
  description = "The ID of the prefix list to modify"
  type        = string
  default = "pl-0416d95afa9ebd533"
}
