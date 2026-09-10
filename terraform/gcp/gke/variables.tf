// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

# us-east-2 for the Transaction Search reason documented in terraform/gcp/gce/variables.tf:
# trace validation reads the aws/spans log group, which only exists where the X-Ray trace
# segment destination is CloudWatchLogs.
variable "region" {
  type    = string
  default = "us-east-2"
}

variable "test_dir" {
  type    = string
  default = "./test/gcp/gke"
}

variable "cwa_github_sha" {
  type    = string
  default = ""
}

# Existing project the cluster is created in (input so CI needs no project create/delete perms).
variable "gcp_project" {
  type    = string
  default = ""
}

# GKE node capacity in us-east1-b has a history of stockouts.
variable "gcp_zone" {
  type    = string
  default = "us-east1-c"
}

# Existing VPC network + subnetwork the cluster attaches to (must exist; no safe default).
variable "gcp_network_name" {
  type    = string
  default = ""
}

variable "gcp_subnetwork_name" {
  type    = string
  default = "default"
}

# Required, not defaulted: the API server is public and this is the only thing scoping it to the runner.
variable "runner_ip" {
  type        = string
  description = "Runner public IP CIDR (e.g. \"1.2.3.4/32\") allowed to reach the GKE API server."
}

variable "gke_node_machine_type" {
  type    = string
  default = "e2-standard-4"
}

variable "gke_node_count" {
  type    = number
  default = 1
}

variable "kubernetes_version" {
  type        = string
  description = "GKE Kubernetes version. null lets GKE pick the default channel's version."
  default     = null
}

variable "cwagent_image_repo" {
  type        = string
  description = "ECR repository URI for the pre-built CWA container image."
}

variable "ecr_region" {
  type        = string
  description = "Region of the integration-test ECR repository (the build publishes to us-west-2 only)."
  default     = "us-west-2"
}

variable "cwagent_image_tag" {
  type        = string
  description = "Image tag (build_id / commit SHA)."
}
