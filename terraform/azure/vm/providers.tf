// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

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
