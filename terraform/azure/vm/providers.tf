// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

# aws is constrained the same way the rest of the repo constrains it. azurerm is major-version pinned
# because it is new to this repo and 5.x carries breaking schema changes relative to the 4.x resources
# used here (unpinning it resolved 5.0.0 and broke the sibling AKS module).
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "!= 6.22.0"
    }
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 4.0"
    }
  }
}

provider "aws" {
  region = var.region
}

# Credentials come from ARM_* environment variables (client id/secret, tenant, subscription) in CI.
provider "azurerm" {
  features {}
}
