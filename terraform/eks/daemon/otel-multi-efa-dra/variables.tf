// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

variable "region" {
  type    = string
  default = "us-west-2"
}

variable "test_dir" {
  type    = string
  default = "./test/otel/multi_efa_dra"
}

variable "cwagent_image_repo" {
  type    = string
  default = "public.ecr.aws/cloudwatch-agent/cloudwatch-agent"
}

variable "cwagent_image_tag" {
  type    = string
  default = "latest"
}

# The chart must render the DRA correlation config (dra_device_types keyed on the
# dra.net driver + the dra.net/rdmaDevice ResourceSlice attribute) and grant the
# agent ServiceAccount get/list/watch on resource.k8s.io resourceclaims and
# resourceslices. Until that lands on main, point helm_chart_repo_url/branch at
# the fork/branch that carries it (helm-charts PR #356).
variable "helm_chart_branch" {
  type    = string
  default = "main"
}

# Repository to clone the observability Helm chart from. Override to a fork when
# validating chart changes that are not yet merged upstream.
variable "helm_chart_repo_url" {
  type    = string
  default = "https://github.com/aws-observability/helm-charts.git"
}

# DRA (resource.k8s.io) is GA (v1) in Kubernetes 1.34, which also still serves
# v1beta1 — the version the processor's DRA informers watch. Keep this at 1.34
# until the processor's DRA client is bumped to v1.
variable "k8s_version" {
  type    = string
  default = "1.34"
}

variable "ami_type" {
  type    = string
  default = "AL2023_x86_64_STANDARD"
}

variable "instance_type" {
  type    = string
  default = "c6in.32xlarge"
}

# dranet Helm chart version (eks/aws-dranet, from https://aws.github.io/eks-charts).
# This is the CHART version (1.0.0); the app version it ships is v1.2.0-eksbuild.2.
variable "dranet_version" {
  type    = string
  default = "1.0.0"
}

variable "efaburn_image" {
  type    = string
  default = "506463145083.dkr.ecr.us-west-2.amazonaws.com/efaburn:latest"
}
