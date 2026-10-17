// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

variable "region" {
  type    = string
  default = "us-west-2"
}

// Stable name for the long-lived cluster. Unlike per-run test clusters, this is
// intentionally not suffixed with a per-run id so the same cluster is reused
// across runs. Provision one per k8s version you need to cover.
variable "cluster_name" {
  type    = string
  default = "cwagent-eks-shared"
}

variable "k8s_version" {
  type    = string
  default = "1.32"
}

variable "ami_type" {
  type    = string
  default = "AL2_x86_64"
}

variable "instance_type" {
  type    = string
  default = "t3a.medium"
}

variable "node_desired_size" {
  type    = number
  default = 2
}

variable "node_min_size" {
  type    = number
  default = 1
}

variable "node_max_size" {
  type    = number
  default = 3
}

variable "cwagent_image_repo" {
  type    = string
  default = "public.ecr.aws/cloudwatch-agent/cloudwatch-agent"
}

variable "cwagent_image_tag" {
  type    = string
  default = "latest"
}
