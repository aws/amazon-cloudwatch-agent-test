// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

variable "region" {
  type    = string
  default = "us-west-2"
}

variable "test_dir" {
  type    = string
  default = "./test/entity"
}

variable "k8s_version" {
  type    = string
  default = "1.30"
}

variable "ami_type" {
  type    = string
  default = "AL2_x86_64"
}

variable "instance_type" {
  type    = string
  default = "t3a.medium"
}

variable "helm_chart_branch" {
  type    = string
  default = "main"
}

variable "cwagent_image_repo" {
  type    = string
  default = "506463145083.dkr.ecr.us-west-2.amazonaws.com/cwagent-integration-test"
}

variable "cwagent_image_tag" {
  type    = string
  default = "896a6b60258cf448fb5ffa57610972e1a4c82826"
}