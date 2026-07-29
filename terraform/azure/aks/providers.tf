// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

# aws is constrained the same way the rest of the repo constrains it. azurerm and kubernetes are
# major-version pinned because they are new to this repo and both have breaking schema changes past
# the majors this module targets: unpinning azurerm resolved 5.0.0, which fails with
# "At least 1 node_provisioning_profile blocks are required" on azurerm_kubernetes_cluster, and
# kubernetes resolved 3.2.1 across its own 2.x -> 3.x break.
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
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.30"
    }
    tls = {
      source  = "hashicorp/tls"
      version = "~> 4.0"
    }
    local = {
      source  = "hashicorp/local"
      version = "~> 2.4"
    }
  }
}

provider "aws" {
  region = var.region
}

provider "aws" {
  alias  = "ecr"
  region = var.ecr_region
}

provider "azurerm" {
  features {}
}

provider "kubernetes" {
  host                   = azurerm_kubernetes_cluster.cwagent.kube_config[0].host
  client_certificate     = base64decode(azurerm_kubernetes_cluster.cwagent.kube_config[0].client_certificate)
  client_key             = base64decode(azurerm_kubernetes_cluster.cwagent.kube_config[0].client_key)
  cluster_ca_certificate = base64decode(azurerm_kubernetes_cluster.cwagent.kube_config[0].cluster_ca_certificate)
}
