// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "common" {
  source             = "../../../common"
  cwagent_image_repo = var.cwagent_image_repo
  cwagent_image_tag  = var.cwagent_image_tag
}

module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 21.0"

  name               = "integ-${module.common.testing_id}"
  kubernetes_version = var.k8s_version

  vpc_id     = aws_vpc.efa_test_vpc.id
  subnet_ids = aws_subnet.efa_test_public_subnet[*].id

  # CloudWatch logging - renamed from cluster_enabled_log_types
  enabled_log_types = ["api", "audit", "authenticator", "controllerManager", "scheduler"]

  eks_managed_node_groups = {
    efa_nodes = {
      # EFA configuration - only at node group level in v21
      enable_efa_support = true
      ami_type           = "AL2_x86_64_GPU"
      instance_types     = [var.instance_type]

      min_size     = 1
      max_size     = 1
      desired_size = 1

      # Use private subnets for nodes
      subnet_ids = aws_subnet.efa_test_private_subnet[*].id

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
    coredns = {}
    eks-pod-identity-agent = {
      before_compute = true
    }
    kube-proxy = {}
    vpc-cni = {
      before_compute = true
    }
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
