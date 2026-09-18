// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

#####################################################################
# AWS side
#####################################################################

# Trace validation reads the aws/spans log group, which only exists where the X-Ray trace segment
# destination is CloudWatchLogs. That destination is a per-region setting, so this suite cannot use the
# repo-wide us-west-2 default: us-west-2 is deliberately left on the legacy XRay destination because the
# App Signals trace suite there validates through the X-Ray query APIs, which Transaction Search would
# break. Matches terraform/azure/vm (linux), the sibling suite that posts to the X-Ray OTLP endpoint.
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

# Local path to the agent .msi (downloaded onto the runner from S3 by the workflow); uploaded to the VM
# over WinRM with a file provisioner. Mirrors the linux agent_deb_path: an Azure VM has no AWS instance
# profile, so it cannot "aws s3 cp" the artifact the way the EC2 Windows suite does.
variable "agent_msi_path" {
  type    = string
  default = ""
}

# Go toolchain version installed on the VM to run the suite. Must match the test repo go.mod "go"
# directive (go.mod:3) so the build uses the declared minimum; bump this if a dependency needs newer.
variable "go_version" {
  type    = string
  default = "1.20"
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

# Local admin account created on the VM. Not "administrator"/"admin" etc., which Azure reserves.
variable "admin_username" {
  type    = string
  default = "cwagent"
}

# CIDR allowed inbound WinRM to the VM (the CI runner's public IP, e.g. "1.2.3.4/32").
# Required, not defaulted: this is the only thing scoping the NSG's Allow-WinRM rule.
variable "runner_ip" {
  type = string
}

# Windows image the VM boots; drives the MSI install path main.tf uses.
variable "azure_image" {
  type = object({
    publisher = string
    offer     = string
    sku       = string
    version   = string
  })
  default = {
    publisher = "MicrosoftWindowsServer"
    offer     = "WindowsServer"
    sku       = "2022-datacenter-azure-edition"
    version   = "latest"
  }
}

# ARN of a pre-created IAM OIDC provider for the Azure AD issuer (https://sts.windows.net/<tenant-id>/).
# Created out-of-band because a newly-created provider for an Azure issuer may be auto-removed if unapproved.
variable "azure_oidc_provider_arn" {
  type    = string
  default = ""
}

# Token audience (aud) the managed identity mints; must match the role trust condition.
variable "azure_token_audience" {
  type    = string
  default = "https://management.azure.com/"
}
