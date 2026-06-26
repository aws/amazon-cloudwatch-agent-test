// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

variable "region" {
  type    = string
  default = "us-west-2"
}

variable "test_dir" {
  type    = string
  default = "./test/otel/pernode"
}

# --- Agent image (DaemonSet). Public by default; per-node needs no agent change. ---
variable "cwagent_image_repo" {
  type    = string
  default = "public.ecr.aws/cloudwatch-agent/cloudwatch-agent"
}

variable "cwagent_image_tag" {
  type    = string
  default = "latest"
}

# --- Helm chart source. MUST point at a checkout that contains the zero-step CRD
#     bundling (G1) changes. Defaults to this repo's fork; override for a PR branch. ---
variable "helm_chart_repo" {
  type    = string
  default = "https://github.com/aws-observability/helm-charts.git"
}

variable "helm_chart_branch" {
  type    = string
  default = "main"
}

# --- Custom operator image: REQUIRED. The public operator hardcodes
#     consistent-hashing and ignores the per-node CR field, so the per-node +
#     CRD-watch (G2) code only runs from a custom build. ---
variable "operator_image_domain" {
  type        = string
  description = "Registry domain for the custom operator image (maps manager.image.repositoryDomainMap.public)."
  # e.g. <account>.dkr.ecr.us-west-2.amazonaws.com
}

variable "operator_image_repo" {
  type        = string
  description = "Repository (path) for the custom operator image, e.g. wenepra/cwagent-test/cloudwatch-agent-operator."
}

variable "operator_image_tag" {
  type = string
}

# --- Custom Target Allocator image: REQUIRED. Patched onto the
#     AmazonCloudWatchAgent CR after install (the chart does not expose it as a
#     first-class value), mirroring scripts/deploy-all.sh CUSTOM_TA. ---
variable "ta_image" {
  type        = string
  description = "Full Target Allocator image ref, e.g. <ecr>/cloudwatch-agent-target-allocator:pernodeN."
}

# --- Allocation strategy under test. ---
variable "allocation_strategy" {
  type    = string
  default = "per-node"
}

variable "k8s_version" {
  type    = string
  default = "1.35"
}

variable "ami_type" {
  type    = string
  default = "AL2023_x86_64_STANDARD"
}

variable "instance_type" {
  type    = string
  default = "t3.medium"
}

# Number of worker nodes. >=2 so per-node spread is meaningful.
variable "node_count" {
  type    = number
  default = 2
}
