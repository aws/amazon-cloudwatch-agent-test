// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "fluent_common" {
  source        = "../common"
  ami_type      = var.ami_type
  instance_type = var.instance_type
}

module "basic_components" {
  source = "../../../../basic_components"

  region = var.region
}

locals {
  aws_eks      = "aws eks --region ${var.region}"
  cluster_name = module.fluent_common.cluster_name
}

data "aws_caller_identity" "account_id" {}

data "aws_eks_cluster" "eks_windows_cluster_ca" {
  name = module.fluent_common.cluster_name
}

output "account_id" {
  value = data.aws_caller_identity.account_id.account_id
}

data "aws_eks_cluster_auth" "this" {
  name = module.fluent_common.cluster_name
}

## EKS Cluster Addon

resource "aws_eks_addon" "eks_windows_addon" {
  cluster_name = module.fluent_common.cluster_name
  addon_name   = "vpc-cni"
}

## Enable VPC CNI Windows Support

resource "kubernetes_config_map_v1_data" "amazon_vpc_cni_windows" {
  depends_on = [
    module.fluent_common,
    aws_eks_addon.eks_windows_addon
  ]
  metadata {
    name      = "amazon-vpc-cni"
    namespace = "kube-system"
  }

  force = true

  data = {
    enable-windows-ipam : "true"
  }
}

## AWS CONFIGMAP

resource "kubernetes_config_map" "configmap" {
  data = {
    "mapRoles" = <<EOT
- groups:
  - system:bootstrappers
  - system:nodes
  rolearn: arn:aws:iam::${data.aws_caller_identity.account_id.account_id}:role/${module.fluent_common.node_role_name}
  username: system:node:{{EC2PrivateDNSName}}
- groups:
  - eks:kube-proxy-windows
  - system:bootstrappers
  - system:nodes
  rolearn: arn:aws:iam::${data.aws_caller_identity.account_id.account_id}:role/${module.fluent_common.node_role_name}
  username: system:node:{{EC2PrivateDNSName}}
- groups:
  - system:masters
  rolearn: arn:aws:iam::${data.aws_caller_identity.account_id.account_id}:role/Admin-Windows

EOT
  }

  metadata {
    name      = "aws-auth"
    namespace = "kube-system"
  }

  lifecycle {
    prevent_destroy = true
  }
}

# EKS Windows Node Groups
resource "aws_eks_node_group" "node_group_windows" {
  cluster_name    = module.fluent_common.cluster_name
  node_group_name = "${local.cluster_name}-windows-node"
  node_role_arn   = module.fluent_common.node_role_arn
  subnet_ids      = module.basic_components.public_subnet_ids

  scaling_config {
    desired_size = 1
    max_size     = 1
    min_size     = 1
  }

  ami_type       = var.windows_ami_type
  capacity_type  = "ON_DEMAND"
  disk_size      = 50
  instance_types = ["t3.large"]

  depends_on = [
    module.fluent_common
  ]
}

resource "null_resource" "kubectl" {
  depends_on = [
    aws_eks_node_group.node_group_windows
  ]
  provisioner "local-exec" {
    command = <<-EOT
      ${local.aws_eks} update-kubeconfig --name ${module.fluent_common.cluster_name}
      ${local.aws_eks} list-clusters --output text
      ${local.aws_eks} describe-cluster --name ${module.fluent_common.cluster_name} --output text
    EOT
  }
}

resource "kubernetes_config_map" "cluster_info" {
  depends_on = [
    module.fluent_common
  ]
  metadata {
    name      = "fluent-bit-cluster-info"
    namespace = "amazon-cloudwatch"
  }
  data = {
    "cluster.name" = module.fluent_common.cluster_name
    "logs.region"  = var.region
    "http.server"  = "Off"
    "http.port"    = "2020"
    "read.head"    = "Off"
    "read.tail"    = "On"
  }
}

resource "kubernetes_service_account" "fluentbit_service" {
  metadata {
    name      = "fluent-bit"
    namespace = "amazon-cloudwatch"
  }
}

resource "kubernetes_cluster_role" "fluentbit_clusterrole" {
  metadata {
    name = "fluent-bit-role"
  }
  rule {
    non_resource_urls = ["/metrics"]
    verbs             = ["get"]
  }
  rule {
    verbs      = ["get", "list", "watch"]
    resources  = ["namespaces", "pods", "pods/logs", "nodes", "nodes/proxy"]
    api_groups = [""]
  }
}

resource "kubernetes_cluster_role_binding" "fluentbit_rolebinding" {
  depends_on = [
    kubernetes_service_account.fluentbit_service,
    kubernetes_cluster_role.fluentbit_clusterrole,
  ]
  metadata {
    name = "fluent-bit-role-binding"
  }
  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "ClusterRole"
    name      = "fluent-bit-role"
  }
  subject {
    kind      = "ServiceAccount"
    name      = "fluent-bit"
    namespace = "amazon-cloudwatch"
  }
}

resource "null_resource" "fluentbit-windows" {
  depends_on = [
    module.fluent_common,
    aws_eks_node_group.node_group_windows,
    null_resource.kubectl
  ]

  provisioner "local-exec" {
    command = <<-EOT
      curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
      chmod +x kubectl
      ./kubectl apply -f ./../../../default_resources/fluenbit-windows-configmap.yaml
      ./kubectl apply -f ./../../../default_resources/fluenbit-windows-daemonset.yaml
      ./kubectl rollout status daemonset fluent-bit-windows -n amazon-cloudwatch --timeout 600s
      sed -e 's+WINDOWS_SERVER_VERSION+${var.windows_os_version}+' -e 's+REPLICAS+1+' ./../../../default_resources/test-sample-windows.yaml | ./kubectl apply -f -
      ./kubectl rollout status deployment windows-test-deployment --timeout 600s
      sleep 120
    EOT
  }
}

resource "null_resource" "validator" {
  depends_on = [
    module.fluent_common,
    null_resource.fluentbit-windows,
    kubernetes_cluster_role_binding.fluentbit_rolebinding
  ]
  provisioner "local-exec" {
    command = <<-EOT
      echo "Validating EKS fluentbit logs"
      cd ../../../../../..
      go test ${var.test_dir} -eksClusterName=${module.fluent_common.cluster_name} -computeType=EKS -v -eksDeploymentStrategy=DAEMON -instancePlatform=windows
    EOT
  }
}

resource "null_resource" "clean-up" {
  depends_on = [
    module.fluent_common,
    null_resource.fluentbit-windows,
    kubernetes_cluster_role_binding.fluentbit_rolebinding,
    null_resource.validator
  ]
  provisioner "local-exec" {
    command = <<-EOT
      echo "Cleaning up EKS fluentbit applications"
      sed -e 's+WINDOWS_SERVER_VERSION+${var.windows_os_version}+' -e 's+REPLICAS+1+' ./../../../default_resources/test-sample-windows.yaml | ./kubectl apply -f -
      ./kubectl wait --for=delete deployment.apps windows-test-deployment --timeout 600s
      ./kubectl delete -f ./../../../default_resources/fluenbit-windows-daemonset.yaml
      ./kubectl wait --for=delete daemonset.apps fluent-bit-windows --timeout 600s
    EOT
  }
}
