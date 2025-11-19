// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

variable "region" {
  type    = string
  default = "us-west-2"
}

variable "test_dir" {
  type    = string
  default = "./test/efa"
}

variable "cwagent_image_repo" {
  type    = string
  default = "public.ecr.aws/cloudwatch-agent/cloudwatch-agent"
}

variable "cwagent_image_tag" {
  type    = string
  default = "latest"
}

variable "k8s_version" {
  type    = string
  default = "1.33"
}

variable "ami_type" {
  type    = string
  default = "AL2023_x86_64_NVIDIA"
}

# NCCL only works on certain P instance types https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/efa-start-nccl.html
variable "instance_type" {
  type    = string
  default = "g4dn.8xlarge"
}