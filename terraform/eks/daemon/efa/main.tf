// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "common" {
  source             = "../../../common"
  cwagent_image_repo = var.cwagent_image_repo
  cwagent_image_tag  = var.cwagent_image_tag
}

module "basic_components" {
  source = "../../../basic_components"

  region = var.region
}

module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 21.0"

  name               = "cwagent-eks-integ-${module.common.testing_id}"
  kubernetes_version = var.k8s_version

  vpc_id     = module.basic_components.vpc_id
  subnet_ids = module.basic_components.public_subnet_ids

  # CloudWatch logging - renamed from cluster_enabled_log_types
  enabled_log_types = ["api", "audit", "authenticator", "controllerManager", "scheduler"]

  eks_managed_node_groups = {
    efa_nodes = {
      # EFA configuration - only at node group level in v21
      enable_efa_support = true
      ami_type           = "AL2023_x86_64_NVIDIA"
      instance_types     = [var.instance_type]

      min_size     = 1
      max_size     = 1
      desired_size = 1

      labels = {
        "vpc.amazonaws.com/efa.present" = "true"
        "nvidia.com/gpu.present"        = "true"
      }

      tags = {
        Owner = "default"
      }
    }
  }

  # EKS Addons - renamed from cluster_addons, most_recent = true is now default
  addons = {
    "amazon-cloudwatch-observability" = {}
  }

  tags = {
    Owner = "default"
  }
}

# Data source for cluster auth (needed for Kubernetes provider)
data "aws_eks_cluster_auth" "this" {
  name = module.eks.cluster_name
}
