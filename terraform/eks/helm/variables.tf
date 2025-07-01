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

variable "cluster_endpoint" {
  type = string
}

variable "cluster_ca_certificate" {
  type = string
}

variable "cluster_auth_token" {
  type = string
  sensitive = true
}

variable "helm_charts_branch" {
  type    = string
  default = "main"
}

variable "cloudwatch_agent_repository" {
  type    = string
  default = "cloudwatch-agent"
}

variable "cloudwatch_agent_tag" {
  type    = string
  default = "latest"
}

variable "cloudwatch_agent_repository_url" {
  type    = string
  default = "public.ecr.aws/cloudwatch-agent"
}

variable "cloudwatch_agent_operator_repository" {
  type    = string
  default = "cloudwatch-agent-operator"
}

variable "cloudwatch_agent_operator_tag" {
  type    = string
  default = "latest"
}

variable "cloudwatch_agent_operator_repository_url" {
  type    = string
  default = "public.ecr.aws/cloudwatch-agent"
}

variable "cloudwatch_agent_target_allocator_repository" {
  type    = string
  default = "cloudwatch-agent-target-allocator"
}

variable "cloudwatch_agent_target_allocator_tag" {
  type    = string
  default = "latest"
}

variable "cloudwatch_agent_target_allocator_repository_url" {
  type    = string
  default = "public.ecr.aws/cloudwatch-agent"
}

variable "test_dir" {
  type    = string
  default = ""
}
