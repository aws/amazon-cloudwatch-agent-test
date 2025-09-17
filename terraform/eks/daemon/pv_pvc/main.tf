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

data "aws_eks_cluster_auth" "this" {
  name = aws_eks_cluster.this.name
}

resource "aws_eks_cluster" "this" {
  name     = "cwagent-eks-integ-${module.common.testing_id}"
  role_arn = module.basic_components.role_arn
  version  = var.k8s_version
  vpc_config {
    subnet_ids         = module.basic_components.public_subnet_ids
    security_group_ids = [module.basic_components.security_group]
  }
}

resource "aws_eks_node_group" "this" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "cwagent-pv-pvc-node-${module.common.testing_id}"
  node_role_arn   = aws_iam_role.node_role.arn
  subnet_ids      = module.basic_components.public_subnet_ids

  scaling_config {
    desired_size = 1
    max_size     = 1
    min_size     = 1
  }

  ami_type       = var.ami_type
  capacity_type  = "ON_DEMAND"
  disk_size      = 20
  instance_types = [var.instance_type]

  depends_on = [
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy,
    aws_iam_role_policy_attachment.node_AmazonEC2ContainerRegistryReadOnly,
    aws_iam_role_policy_attachment.node_CloudWatchAgentServerPolicy
  ]
}

resource "aws_iam_role" "node_role" {
  name = "cwagent-pv-pvc-Worker-Role-${module.common.testing_id}"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ec2.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy_attachment" "node_AmazonEKSWorkerNodePolicy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy"
  role       = aws_iam_role.node_role.name
}

resource "aws_iam_role_policy_attachment" "node_AmazonEC2ContainerRegistryReadOnly" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
  role       = aws_iam_role.node_role.name
}

resource "aws_iam_role_policy_attachment" "node_CloudWatchAgentServerPolicy" {
  policy_arn = "arn:aws:iam::aws:policy/CloudWatchAgentServerPolicy"
  role       = aws_iam_role.node_role.name
}

# Minimal PV/PVC setup for testing metrics
resource "kubernetes_namespace" "test_ns" {
  depends_on = [aws_eks_node_group.this]
  metadata {
    name = "pv-pvc-test"
  }
}

# Create PVs in different states
resource "kubernetes_persistent_volume" "test_pv_bound" {
  depends_on = [kubernetes_namespace.test_ns]
  metadata {
    name = "test-pv-bound"
  }
  spec {
    capacity     = { storage = "1Gi" }
    access_modes = ["ReadWriteOnce"]
    persistent_volume_source {
      host_path { path = "/tmp/pv-bound" }
    }
  }
}

resource "kubernetes_persistent_volume" "test_pv_available" {
  depends_on = [kubernetes_namespace.test_ns]
  metadata {
    name = "test-pv-available"
  }
  spec {
    capacity     = { storage = "1Gi" }
    access_modes = ["ReadWriteOnce"]
    persistent_volume_source {
      host_path { path = "/tmp/pv-available" }
    }
  }
}

# Create PVCs in different states
resource "kubernetes_persistent_volume_claim" "test_pvc_bound" {
  depends_on = [kubernetes_persistent_volume.test_pv_bound]
  metadata {
    name      = "test-pvc-bound"
    namespace = "pv-pvc-test"
  }
  spec {
    access_modes = ["ReadWriteOnce"]
    resources {
      requests = { storage = "1Gi" }
    }
    volume_name = kubernetes_persistent_volume.test_pv_bound.metadata[0].name
  }
}

resource "kubernetes_persistent_volume_claim" "test_pvc_pending" {
  depends_on = [kubernetes_namespace.test_ns]
  metadata {
    name      = "test-pvc-pending"
    namespace = "pv-pvc-test"
  }
  spec {
    access_modes       = ["ReadWriteOnce"]
    storage_class_name = "non-existent-storage-class" # This will naturally stay pending
    resources {
      requests = { storage = "1Gi" }
    }
  }
}

# Create a PV that will be deleted to simulate Lost status
resource "kubernetes_persistent_volume" "test_pv_to_delete" {
  depends_on = [kubernetes_namespace.test_ns]
  metadata {
    name = "test-pv-to-delete"
  }
  spec {
    capacity     = { storage = "1Gi" }
    access_modes = ["ReadWriteOnce"]
    persistent_volume_source {
      host_path { path = "/tmp/pv-to-delete" }
    }
  }
}

resource "kubernetes_persistent_volume_claim" "test_pvc_will_be_lost" {
  depends_on = [kubernetes_persistent_volume.test_pv_to_delete]
  metadata {
    name      = "test-pvc-will-be-lost"
    namespace = "pv-pvc-test"
  }
  spec {
    access_modes = ["ReadWriteOnce"]
    resources {
      requests = { storage = "1Gi" }
    }
    volume_name = kubernetes_persistent_volume.test_pv_to_delete.metadata[0].name
  }
}

# Job to simulate Lost status by deleting the PV after PVC is bound
resource "kubernetes_job" "create_lost_pvc" {
  depends_on = [kubernetes_persistent_volume_claim.test_pvc_will_be_lost]
  metadata {
    name      = "create-lost-pvc"
    namespace = "pv-pvc-test"
  }
  spec {
    template {
      metadata {}
      spec {
        restart_policy = "Never"
        container {
          name    = "kubectl"
          image   = "bitnami/kubectl:latest"
          command = ["/bin/bash", "-c"]
          args = [
            # Wait for PVC to bind, remove finalizers, then delete PV to create Lost status
            "kubectl wait --for=condition=Bound pvc/test-pvc-will-be-lost --timeout=30s && sleep 5 && kubectl patch pvc test-pvc-will-be-lost -p '{\"metadata\":{\"finalizers\":null}}' && kubectl delete pv test-pv-to-delete --force --grace-period=0"
          ]
        }
        service_account_name = "pvc-manager"
      }
    }
  }
}

# Service account and RBAC for the job
resource "kubernetes_service_account" "pvc_manager" {
  depends_on = [kubernetes_namespace.test_ns]
  metadata {
    name      = "pvc-manager"
    namespace = "pv-pvc-test"
  }
}

resource "kubernetes_cluster_role" "pvc_manager_role" {
  depends_on = [kubernetes_namespace.test_ns]
  metadata {
    name = "pvc-manager-role"
  }
  rule {
    api_groups = [""]
    resources  = ["persistentvolumes", "persistentvolumeclaims"]
    verbs      = ["get", "list", "watch", "delete", "patch"]
  }
}

resource "kubernetes_cluster_role_binding" "pvc_manager_binding" {
  depends_on = [kubernetes_cluster_role.pvc_manager_role]
  metadata {
    name = "pvc-manager-binding"
  }
  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "ClusterRole"
    name      = "pvc-manager-role"
  }
  subject {
    kind      = "ServiceAccount"
    name      = "pvc-manager"
    namespace = "pv-pvc-test"
  }
}

# Install CloudWatch Agent via helm
data "external" "clone_helm_chart" {
  program = ["bash", "-c", <<-EOT
    rm -rf ./helm-charts
    git clone -b ${var.helm_chart_branch} https://github.com/aws-observability/helm-charts.git ./helm-charts
    echo '{"status":"ready"}'
  EOT
  ]
}

resource "helm_release" "aws_observability" {
  name             = "amazon-cloudwatch-observability"
  chart            = "./helm-charts/charts/amazon-cloudwatch-observability"
  namespace        = "amazon-cloudwatch"
  create_namespace = true

  set = [
    {
      name  = "clusterName"
      value = aws_eks_cluster.this.name
    },
    {
      name  = "region"
      value = var.region
    }
  ]
  depends_on = [
    aws_eks_cluster.this,
    aws_eks_node_group.this,
    data.external.clone_helm_chart,
  ]
}

resource "null_resource" "update_image" {
  depends_on = [helm_release.aws_observability]
  provisioner "local-exec" {
    command = <<-EOT
      aws eks --region ${var.region} update-kubeconfig --name ${aws_eks_cluster.this.name}
      kubectl -n amazon-cloudwatch patch AmazonCloudWatchAgent cloudwatch-agent --type='json' -p='[{"op": "replace", "path": "/spec/image", "value": "${var.cwagent_image_repo}:${var.cwagent_image_tag}"}]'
      sleep 10
    EOT
  }
}

resource "null_resource" "validator" {
  depends_on = [
    kubernetes_persistent_volume_claim.test_pvc_bound,
    kubernetes_persistent_volume_claim.test_pvc_pending,
    kubernetes_job.create_lost_pvc,
    helm_release.aws_observability,
    null_resource.update_image,
  ]

  provisioner "local-exec" {
    command = <<-EOT
      cd ../../../..
      # Wait a bit for the Lost PVC state to be created
      sleep 30
      go test ${var.test_dir} -timeout 1h -eksClusterName=${aws_eks_cluster.this.name} -computeType=EKS -v -eksDeploymentStrategy=DAEMON
    EOT
  }
}
