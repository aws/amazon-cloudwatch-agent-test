// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

variable "region" {
  type    = string
  default = "us-west-2"
}

variable "test_dir" {
  type    = string
  default = "./test/otel/gpu_dra"
}

variable "cwagent_image_repo" {
  type    = string
  default = "public.ecr.aws/cloudwatch-agent/cloudwatch-agent"
}

variable "cwagent_image_tag" {
  type    = string
  default = "latest"
}

# The chart must render the DRA correlation config (dra_device_types incl. the
# gpu-dra entry keyed on the gpu.nvidia.com driver) and grant the agent
# ServiceAccount resource.k8s.io RBAC. Override to a fork/branch until that lands
# on main (helm-charts PR #356).
variable "helm_chart_branch" {
  type    = string
  default = "main"
}

variable "helm_chart_repo_url" {
  type    = string
  default = "https://github.com/aws-observability/helm-charts.git"
}

# DRA (resource.k8s.io) is GA (v1) in k8s 1.34, which still serves v1beta1 — the
# version the processor's DRA informers watch. Keep at 1.34 until the client is
# bumped to v1.
variable "k8s_version" {
  type    = string
  default = "1.34"
}

variable "ami_type" {
  type    = string
  default = "AL2023_x86_64_NVIDIA"
}

# Multi-GPU node: g4dn.12xlarge = 4 T4 GPUs.
variable "instance_type" {
  type    = string
  default = "g4dn.12xlarge"
}

# NVIDIA DRA driver Helm chart (DeviceClass gpu.nvidia.com).
# Repo: https://helm.ngc.nvidia.com/nvidia, chart: nvidia-dra-driver-gpu.
variable "nvidia_dra_repo" {
  type    = string
  default = "https://helm.ngc.nvidia.com/nvidia"
}

variable "nvidia_dra_chart_version" {
  type    = string
  default = ""
}
