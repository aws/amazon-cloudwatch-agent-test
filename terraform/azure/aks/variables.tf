// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

variable "region" {
  type    = string
  default = "us-east-2"
}

# Selects the suite deployed through the amazon-cloudwatch-observability chart:
#   "./test/azure/aks"                   -> default:otel agent + OTLP load generator
#   "./test/azure/aks/containerinsights" -> OTEL Container Insights node agent + cluster-scraper
#   "./test/azure/aks/gpu"               -> OTEL Container Insights + DCGM on an AKS-managed GPU pool
#   "./test/azure/aks/gpudriver"         -> OTEL Container Insights + DCGM from the chart's exporter on driver-install GPU pools
variable "test_dir" {
  type    = string
  default = "./test/azure/aks"
  validation {
    condition     = contains(["./test/azure/aks", "./test/azure/aks/containerinsights", "./test/azure/aks/standard", "./test/azure/aks/gpu", "./test/azure/aks/gpudriver"], var.test_dir)
    error_message = "test_dir must be \"./test/azure/aks\", \"./test/azure/aks/containerinsights\", \"./test/azure/aks/standard\", \"./test/azure/aks/gpu\" or \"./test/azure/aks/gpudriver\"."
  }
}

variable "cwa_github_sha" {
  type    = string
  default = ""
}

variable "azure_location" {
  type    = string
  default = "westus"
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

variable "aks_gpu_vm_size" {
  type        = string
  description = "VM size of the gpumanaged pool (gpu suite) and the gpuidle pool (gpudriver suite)."
  default     = "Standard_NC4as_T4_v3"
}

variable "aks_gpu_burn_vm_size" {
  type        = string
  description = "VM size of the gpuburn pool in the gpudriver suite (the EKS gpu_multi node group)."
  default     = "Standard_NC4as_T4_v3"
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

variable "helm_chart_branch" {
  type    = string
  default = "main"
}

variable "helm_chart_repo" {
  type = string
  # TEMPORARY: dhwanianjirwala's fork carries aws-observability/helm-charts PR #387 (DCGM on AKS,
  # branch fix/aks-dcgm-exporter). Switch back to the aws-observability URL once #387 is merged.
  description = "Git URL of the helm-charts repository to clone. Pair it with helm_chart_branch."
  default     = "https://github.com/dhwanianjirwala/helm-charts.git"
}

variable "helm_chart_path" {
  type        = string
  description = "Local amazon-cloudwatch-observability chart directory. When set, it is installed instead of cloning helm_chart_branch."
  default     = ""
}

variable "helm_set_values" {
  type        = map(string)
  description = "Additional chart values applied after the suite's own values."
  default     = {}
}
