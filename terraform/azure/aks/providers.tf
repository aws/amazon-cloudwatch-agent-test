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
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.30"
    }
    tls = {
      source  = "hashicorp/tls"
      version = "~> 4.0"
    }
  }
}

provider "aws" {
  region = var.region
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
