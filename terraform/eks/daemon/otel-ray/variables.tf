// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

variable "region" {
  type    = string
  default = "us-west-2"
}

variable "test_dir" {
  type    = string
  default = "./test/otel/solutions/ray"
}

variable "cwagent_image_repo" {
  type    = string
  default = "public.ecr.aws/cloudwatch-agent/cloudwatch-agent"
}

variable "cwagent_image_tag" {
  type    = string
  default = "latest"
}

# The chart is cloned from helm_chart_repo at helm_chart_branch, so a branch on
# a fork can be tested before it is merged.
variable "helm_chart_repo" {
  type    = string
  default = "https://github.com/aws-observability/helm-charts.git"
}

variable "helm_chart_branch" {
  type    = string
  default = "main"
}

variable "k8s_version" {
  type    = string
  default = "1.35"
}

variable "ami_type" {
  type    = string
  default = "AL2023_x86_64_STANDARD"
}

# A Ray head and a worker, each with a CPU and a few GiB, plus the agent.
variable "instance_type" {
  type    = string
  default = "m5.xlarge"
}

variable "kuberay_version" {
  type    = string
  default = "1.7.1"
}

variable "ray_version" {
  type    = string
  default = "2.59.0"
}

variable "ray_image" {
  type    = string
  default = "rayproject/ray:2.59.0-py311-cpu"
}
