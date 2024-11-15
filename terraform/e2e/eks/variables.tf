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
  default = "cwagent-otel-config-e2e-eks"
}

variable "agent_branch" {
  type    = string
  default = "main"
}

variable "operator_branch" {
  type    = string
  default = "main"
}

variable "helm_charts_branch" {
  type    = string
  default = "main"
}

variable "otel-config" {
  type    = string
  default = ""
}

variable "agent-config" {
  type    = string
  default = "../../../test/e2e/jmx/files/cwagent_configs/jvm_tomcat.json"
}

variable "prometheus-config" {
  type    = string
  default = ""
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

variable "validate_test" {
  type    = string
  default = "../../../test/e2e/jmx/validate_test.go"
}

variable "sample-app" {
  type = string
  default = "../../../test/e2e/jmx/files/sample_applications/tomcat-deployment.yaml"
}

variable "sample-app-name" {
  type = string
  default = "tomcat-deployment"
}