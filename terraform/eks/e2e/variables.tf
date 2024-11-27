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
  default = "cwagent-monitoring-config-e2e-eks"
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

variable "test_dir" {
  type    = string
  default = ""
}

variable "agent_config" {
  type    = string
  default = ""
}

variable "otel_config" {
  type    = string
  default = ""
}

variable "prometheus_config" {
  type    = string
  default = ""
}

variable "sample_app" {
  type    = string
  default = ""
}
