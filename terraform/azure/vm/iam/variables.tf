// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

# Name prefix for the IAM role/policy, e.g. "cwa-azurevm-integ" (linux) or "cwa-azurevmwin-integ" (win).
variable "name_prefix" {
  type = string
}

# Unique per-run suffix (module.common.testing_id in the root) so parallel runs never collide.
variable "testing_id" {
  type = string
}

# The VM's system-assigned managed identity principal id, pinned as the trust policy :sub condition.
variable "principal_id" {
  type = string
}

variable "azure_oidc_provider_arn" {
  type = string
}

variable "azure_token_audience" {
  type = string
}
