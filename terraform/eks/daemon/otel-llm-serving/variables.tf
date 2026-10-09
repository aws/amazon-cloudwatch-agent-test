// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

variable "region" {
  type    = string
  default = "us-west-2"
}

# Run first; ./test/otel/solutions/{kserve,knative} always follow.
variable "test_dir" {
  type    = string
  default = "./test/otel/solutions/vllm"
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

# vLLM runs on CPU here; two servers plus the KServe/Knative/Istio control
# planes need more than the t3.medium the other OTEL suites use.
variable "instance_type" {
  type    = string
  default = "m5.2xlarge"
}

variable "cert_manager_version" {
  type    = string
  default = "v1.17.0"
}

variable "istio_version" {
  type    = string
  default = "1.27.1"
}

variable "knative_version" {
  type    = string
  default = "1.21.1"
}

variable "knative_net_istio_version" {
  type    = string
  default = "1.21.4"
}

variable "kserve_version" {
  type    = string
  default = "v0.20.0"
}

variable "vllm_cpu_image" {
  type    = string
  default = "public.ecr.aws/q9t5s3a7/vllm-cpu-release-repo:v0.29.0"
}

variable "vllm_model" {
  type    = string
  default = "Qwen/Qwen2.5-0.5B-Instruct"
}
