// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

#####################################################################
# AWS side
#####################################################################

variable "region" {
  type    = string
  default = "us-west-2"
}

variable "test_dir" {
  type    = string
  default = "./test/azurevm"
}

variable "test_name" {
  type    = string
  default = "azurevm"
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

# CIDR allowed inbound SSH to the VM (the CI runner's public IP, e.g. "1.2.3.4/32"); no default so it must be set.
variable "runner_ip" {
  type    = string
  default = ""
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
