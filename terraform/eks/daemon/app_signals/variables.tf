// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

variable "region" {
  type    = string
  default = "us-west-2"
}

variable "test_dir" {
  type    = string
  default = "./test/app_signals"
}

variable "cwagent_image_repo" {
  type    = string
  //Todo: change this to public ECR repo image after GA "public.ecr.aws/cloudwatch-agent/cloudwatch-agent"
  default = "506463145083.dkr.ecr.us-west-2.amazonaws.com/apm-beta-pre-release"
}

variable "cwagent_image_tag" {
  type    = string
  default = "appsignals"
}

variable "k8s_version" {
  type    = string
  default = "1.24"
}

variable "ami_type" {
  type    = string
  default = "AL2_x86_64"
}

variable "instance_type" {
  type    = string
  default = "t3a.medium"
}