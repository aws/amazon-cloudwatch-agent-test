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
  }
}

provider "aws" {
  region = var.region
}

# Credentials come from GOOGLE_APPLICATION_CREDENTIALS or gcloud application-default
# credentials; project and zone are explicit module inputs.
provider "google" {
  project = var.gcp_project
  zone    = var.gcp_zone
}
