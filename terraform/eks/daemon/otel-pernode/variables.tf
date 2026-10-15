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

# --- Optional agent image override (DaemonSet). Empty = the chart's pinned agent
#     image, which is what ships; per-node needs no agent change. Set all three to
#     test a custom agent build. ---
variable "cwagent_image_domain" {
  type        = string
  default     = ""
  description = "Registry domain for a custom agent image (agent.image.repositoryDomainMap)."
}

variable "cwagent_image_repo" {
  type        = string
  default     = ""
  description = "Repository for a custom agent image (agent.image.repository)."
}

variable "cwagent_image_tag" {
  type        = string
  default     = ""
  description = "Tag for a custom agent image (agent.image.tag)."
}

# --- Helm chart source. MUST resolve to a chart that contains the zero-step CRD
#     bundling (G1), per-node, and routing changes. Upstream `main` does NOT have
#     them until this stack merges, so either set helm_chart_branch to the merge
#     commit/release tag once it lands, or use local_chart_path below to test a
#     working-tree checkout. Pin to a fixed ref (not a moving branch) for CI. ---
variable "helm_chart_repo" {
  type    = string
  default = "https://github.com/aws-observability/helm-charts.git"
}

variable "helm_chart_branch" {
  type    = string
  default = "main"
}

# Install from a LOCAL chart checkout instead of git-cloning helm_chart_repo.
# Useful to test working-tree changes that are not yet committed/pushed. When
# set (absolute path to .../charts/amazon-cloudwatch-observability), the clone is
# skipped. Leave empty for CI (clone).
variable "local_chart_path" {
  type    = string
  default = ""
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
  description = "Repository (path) for the custom operator image, e.g. <org>/cloudwatch-agent-operator."
}

variable "operator_image_tag" {
  type = string
}

# --- Custom Target Allocator image: REQUIRED until a release carries per-node,
#     CRD-watch and scraper_role support. Set through the chart's
#     agent.prometheus.targetAllocator.image values, which every TA the chart
#     renders (per-node agent and cluster-scraper) uses. ---
variable "ta_image_domain" {
  type        = string
  description = "Registry domain for the custom Target Allocator image (agent.prometheus.targetAllocator.image.repositoryDomainMap)."
}

variable "ta_image_repo" {
  type        = string
  description = "Repository for the custom Target Allocator image, e.g. <org>/cloudwatch-agent-target-allocator."
}

variable "ta_image_tag" {
  type = string
}

# --- Optional allocation strategy override. Empty = the chart default (per-node),
#     so the suite covers the chart as shipped. ---
variable "allocation_strategy" {
  type    = string
  default = ""
}

variable "k8s_version" {
  type = string
  # Pinned to a broadly-available GA EKS version. Bump only to versions GA in the
  # target region; a not-yet-offered version makes the default apply fail.
  default = "1.31"
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
