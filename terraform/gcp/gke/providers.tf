// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "!= 6.22.0"
    }
    google = {
      source  = "hashicorp/google"
      version = "~> 6.0"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.0"
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

# Credentials come from GOOGLE_APPLICATION_CREDENTIALS or gcloud application-default
# credentials; project and zone are explicit module inputs.
provider "google" {
  project = var.gcp_project
  zone    = var.gcp_zone
}

# The same ADC identity's bearer token authenticates directly against the cluster
# endpoint, so the machine running terraform needs no exec-based kubectl auth plugin.
data "google_client_config" "current" {}

provider "kubernetes" {
  host                   = "https://${google_container_cluster.cwagent.endpoint}"
  token                  = data.google_client_config.current.access_token
  cluster_ca_certificate = base64decode(google_container_cluster.cwagent.master_auth[0].cluster_ca_certificate)
}
