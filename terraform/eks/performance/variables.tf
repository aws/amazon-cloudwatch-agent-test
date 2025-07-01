// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

variable "region" {
  type    = string
  default = "us-west-2"
}

variable "k8s_version" {
  type    = string
  default = "1.31"
}

variable "cluster_name" {
  type    = string
  default = "cwagent-eks-performance"
}

variable "nodes" {
  type    = number
  default = 1
}

variable "ami_type" {
  type    = string
  default = "AL2_x86_64"
}

variable "instance_type" {
  type    = string
  default = "t3a.medium"
}

variable "test_dir" {
  type    = string
  default = ""
}
