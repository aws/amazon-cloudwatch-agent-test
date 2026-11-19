// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

provider "aws" {
  region = var.region
}

provider "kubernetes" {
  exec {
    api_version = "client.authentication.k8s.io/v1beta1"
    command     = "aws"
    args        = ["eks", "get-token", "--cluster-name", module.fluent_common.cluster_name]
  }
  host                   = module.fluent_common.cluster_endpoint
  cluster_ca_certificate = base64decode(module.fluent_common.cluster_cert)
  token                  = module.fluent_common.cluster_auth_token
}