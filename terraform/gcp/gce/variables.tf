// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

#####################################################################
# AWS side
#####################################################################

# Trace validation reads the aws/spans log group, which only exists where the X-Ray trace segment
# destination is CloudWatchLogs. That destination is a per-region setting, so this suite cannot use the
# repo-wide us-west-2 default: us-west-2 is deliberately left on the legacy XRay destination because the
# App Signals trace suite there validates through the X-Ray query APIs, which Transaction Search would
# break.
variable "region" {
  type    = string
  default = "us-east-2"
}

variable "test_dir" {
  type    = string
  default = "./test/gcp/gce"
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
# GCP side
#####################################################################

# Existing project the VM is created in (input so CI needs no project create/delete perms).
variable "gcp_project" {
  type    = string
  default = ""
}

variable "gcp_zone" {
  type    = string
  default = "us-east1-b"
}

variable "gcp_machine_type" {
  type    = string
  default = "e2-standard-2"
}

# Existing VPC network + subnetwork the VM's NIC attaches to (must exist; no safe default).
variable "gcp_network_name" {
  type    = string
  default = ""
}

variable "gcp_subnetwork_name" {
  type    = string
  default = "default"
}

variable "admin_username" {
  type    = string
  default = "cwagent"
}

# CIDR allowed inbound SSH to the VM (the CI runner's public IP, e.g. "1.2.3.4/32").
# Required, not defaulted: this is the only thing scoping the firewall rule's Allow-22 reach.
variable "runner_ip" {
  type = string
}

# Ubuntu image the VM boots; matches the Debian-package install path main.tf uses.
variable "gcp_image" {
  type    = string
  default = "ubuntu-os-cloud/ubuntu-2404-lts-amd64"
}

# Token audience (aud) requested from the metadata identity endpoint; must match the role trust
# condition. sts.amazonaws.com is the audience the agent's own token provider requests.
variable "gcp_token_audience" {
  type    = string
  default = "sts.amazonaws.com"
}
