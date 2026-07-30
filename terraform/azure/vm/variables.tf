// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

#####################################################################
# AWS side
#####################################################################

# Trace validation reads the aws/spans log group, which only exists where the X-Ray trace segment
# destination is CloudWatchLogs. That destination is a per-region setting, so this suite cannot use the
# repo-wide us-west-2 default: us-west-2 is deliberately left on the legacy XRay destination because the
# App Signals trace suite there validates through the X-Ray query APIs, which Transaction Search would
# break. Matches terraform/azure/aks, the other suite that posts to the X-Ray OTLP endpoint.
variable "region" {
  type    = string
  default = "us-east-2"
}

variable "test_dir" {
  type    = string
  default = "./test/azure/vm"
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

# Local path to the agent .deb (built on the runner); uploaded to the VM over SSH, so no S3/public URL needed.
variable "agent_deb_path" {
  type    = string
  default = ""
}

#####################################################################
# Azure side
#####################################################################

variable "azure_location" {
  type    = string
  default = "eastus"
}

variable "azure_vm_size" {
  type    = string
  default = "Standard_D2s_v7"
}

# Existing resource group the VM is created in (input so CI needs no RG create/delete perms).
variable "azure_resource_group" {
  type    = string
  default = ""
}

# Existing vnet + subnet the VM's NIC attaches to (must exist; no safe default).
variable "azure_vnet_name" {
  type    = string
  default = ""
}

variable "azure_subnet_name" {
  type    = string
  default = "default"
}

variable "admin_username" {
  type    = string
  default = "cwagent"
}

# CIDR allowed inbound SSH to the VM (the CI runner's public IP, e.g. "1.2.3.4/32").
# Required, not defaulted: this is the only thing scoping the NSG's Allow-22 rule.
variable "runner_ip" {
  type = string
}

# Ubuntu image the VM boots; matches the Debian-package install path used below.
variable "azure_image" {
  type = object({
    publisher = string
    offer     = string
    sku       = string
    version   = string
  })
  default = {
    publisher = "Canonical"
    offer     = "ubuntu-24_04-lts"
    sku       = "server"
    version   = "latest"
  }
}

# ARN of a pre-created IAM OIDC provider for the Azure AD issuer (https://sts.windows.net/<tenant-id>/); see the README.
variable "azure_oidc_provider_arn" {
  type    = string
  default = ""
}

# Token audience (aud) the managed identity mints; must match the role trust condition.
variable "azure_token_audience" {
  type    = string
  default = "https://management.azure.com/"
}
