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

variable "github_test_repo" {
  type    = string
  default = "https://github.com/aws/amazon-cloudwatch-agent-test.git"
}

variable "github_test_repo_branch" {
  type    = string
  default = "main"
}

variable "agent_deb_path" {
  type        = string
  description = "Local path to the pre-built agent .deb (uploaded into the DaemonSet container image)."
  default     = ""
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

variable "azure_oidc_provider_arn" {
  type        = string
  description = "Pre-created AWS IAM OIDC provider ARN for the Azure AD issuer (VM test uses this; AKS creates its own)."
  default     = ""
}

variable "azure_token_audience" {
  type    = string
  default = "https://management.azure.com/"
}

variable "runner_ip" {
  type        = string
  description = "Runner public IP for NSG and kubectl access."
  default     = ""
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

variable "ecr_docker_config_json" {
  type        = string
  sensitive   = true
  description = "Unused. The pull secret is generated in terraform via aws_ecr_authorization_token; kept so callers passing this variable do not error."
  default     = ""
}
