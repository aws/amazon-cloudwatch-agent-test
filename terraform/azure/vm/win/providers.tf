// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

# aws matches terraform/eks/daemon/efa. azurerm has no precedent in this repo and is pinned to 4.x
# because 5.0.0 carries breaking schema changes relative to the 4.x resources used here.
# random is used to generate the Windows VM admin password (Windows VMs use password auth, not the
# SSH keypair the linux sibling uses).
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
    random = {
      source  = "hashicorp/random"
      version = "~> 3.0"
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
