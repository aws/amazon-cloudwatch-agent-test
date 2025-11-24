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

variable "nodes" {
  type    = number
  default = 2
}

variable "ami_type" {
  type    = string
  default = "AL2023_x86_64_STANDARD"
}

variable "instance_type" {
  type    = string
  default = "t3a.medium"
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

variable "eks_deployment_strategy" {
  type    = string
  default = "DAEMON"
}

variable "addon_name" {
  type    = string
  default = "amazon-cloudwatch-observability"
}

variable "eks_installation_type" {
  type    = string
  default = "HELM_CHART"
  validation {
    condition     = contains(["HELM_CHART", "EKS_ADDON"], var.eks_installation_type)
    error_message = "eks_installation_type must be either 'HELM_CHART' or 'EKS_ADDON'"
  }
}

variable "vpc_name" {
  type    = string
  default = ""
  description = "Name of an existing VPC to use. If empty, uses default VPC. For IPv6 testing, use 'eksctl-ipv6-eks-nodes-cluster/VPC'"
}

variable "ip_family" {
  type    = string
  default = "ipv4"
  validation {
    condition     = contains(["ipv4", "ipv6"], var.ip_family)
    error_message = "ip_family must be either 'ipv4' or 'ipv6'"
  }
  description = "IP family for the EKS cluster. Use 'ipv6' for dual-stack clusters"
}