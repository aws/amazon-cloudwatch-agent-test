// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

variable "region" {
  type    = string
  default = "us-east-2"
}

variable "test_dir" {
  type    = string
  default = "./test/azure/aks"
}

variable "cwa_github_sha" {
  type    = string
  default = ""
}

variable "azure_location" {
  type    = string
  default = "eastus"
}

variable "azure_resource_group" {
  type = string
}

variable "azure_vnet_name" {
  type = string
}

variable "azure_subnet_name" {
  type    = string
  default = "default"
}

# Required, not defaulted: the API server is public and this is the only thing scoping it to the runner.
variable "runner_ip" {
  type        = string
  description = "Runner public IP CIDR (e.g. \"1.2.3.4/32\") allowed to reach the AKS API server."
}

variable "aks_node_vm_size" {
  type    = string
  default = "Standard_D4s_v7"
}

variable "aks_node_count" {
  type    = number
  default = 1
}

variable "kubernetes_version" {
  type        = string
  description = "AKS Kubernetes version. null lets AKS pick the region's default supported GA version."
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
