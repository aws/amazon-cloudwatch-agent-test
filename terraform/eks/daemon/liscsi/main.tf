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

locals {
  aws_eks = "aws eks --region ${var.region}"
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
  node_group_name = "cwagent-liscsi-eks-integ-node-${module.common.testing_id}"
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
    aws_iam_role_policy_attachment.node_AmazonEC2ContainerRegistryReadOnly,
    aws_iam_role_policy_attachment.node_AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy,
    aws_iam_role_policy_attachment.node_CloudWatchAgentServerPolicy
  ]
}

resource "aws_iam_role" "node_role" {
  name = "cwagent-liscsi-eks-Worker-Role-${module.common.testing_id}"

  assume_role_policy = <<POLICY
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Service": "ec2.amazonaws.com"
      },
      "Action": "sts:AssumeRole"
    }
  ]
}
POLICY
}

resource "aws_iam_role_policy_attachment" "node_AmazonEKSWorkerNodePolicy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy"
  role       = aws_iam_role.node_role.name
}

resource "aws_iam_role_policy_attachment" "node_AmazonEKS_CNI_Policy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy"
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

# LIS CSI addon
resource "aws_eks_addon" "lis_csi_addon" {
  cluster_name         = aws_eks_cluster.this.name
  addon_name           = "aws-ec2-local-instance-store-csi-driver"
  addon_version        = var.lis_csi_addon_version
  configuration_values = jsonencode({ metrics = { enabled = true } })

  depends_on = [
    aws_eks_node_group.this,
  ]
}

resource "null_resource" "kubectl" {
  depends_on = [aws_eks_cluster.this, aws_eks_node_group.this]
  provisioner "local-exec" {
    command = <<-EOT
      ${local.aws_eks} update-kubeconfig --name ${aws_eks_cluster.this.name}
    EOT
  }
}

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

  set {
    name  = "clusterName"
    value = aws_eks_cluster.this.name
  }

  set {
    name  = "region"
    value = var.region
  }
  depends_on = [aws_eks_cluster.this, aws_eks_node_group.this, data.external.clone_helm_chart]
}

resource "null_resource" "update_image" {
  depends_on = [helm_release.aws_observability, null_resource.kubectl]
  triggers = {
    timestamp = "${timestamp()}"
  }
  provisioner "local-exec" {
    command = <<-EOT
      kubectl -n amazon-cloudwatch patch AmazonCloudWatchAgent cloudwatch-agent --type='json' -p='[{"op": "replace", "path": "/spec/image", "value": "${var.cwagent_image_repo}:${var.cwagent_image_tag}"}]'
      sleep 10
    EOT
  }
}

resource "null_resource" "wait_for_lis_csi" {
  depends_on = [aws_eks_addon.lis_csi_addon, null_resource.kubectl]
  provisioner "local-exec" {
    command = <<-EOT
      kubectl rollout status daemonset/ec2-instance-store-plugin -n kube-system --timeout=300s
    EOT
  }
}

resource "kubernetes_persistent_volume_claim" "lis_csi_pvc" {
  metadata {
    name      = "liscsi-integ-test-pvc"
    namespace = "default"
  }
  spec {
    access_modes       = ["ReadWriteOnce"]
    storage_class_name = "ec2-instance-store-sc"
    resources {
      requests = {
        storage = "1Gi"
      }
    }
  }

  depends_on = [null_resource.wait_for_lis_csi]
}

resource "kubernetes_deployment" "lis_csi_io_workload" {
  metadata {
    name      = "liscsi-integ-test-io-workload"
    namespace = "default"
    labels = {
      app = "liscsi-integ-test"
    }
  }
  spec {
    replicas = 1
    selector {
      match_labels = {
        app = "liscsi-integ-test"
      }
    }
    template {
      metadata {
        labels = {
          app = "liscsi-integ-test"
        }
      }
      spec {
        container {
          name    = "liscsi-integ-test-writer"
          image   = "busybox:1.35"
          command = ["sh", "-c", "while true; do dd if=/dev/zero of=/data/out.txt bs=1M count=10; sleep 5; done"]
          volume_mount {
            name       = "lis-storage"
            mount_path = "/data"
          }
        }
        volume {
          name = "lis-storage"
          persistent_volume_claim {
            claim_name = kubernetes_persistent_volume_claim.lis_csi_pvc.metadata[0].name
          }
        }
      }
    }
  }

  depends_on = [kubernetes_persistent_volume_claim.lis_csi_pvc, null_resource.kubectl]
}

data "aws_instances" "eks_node" {
  depends_on = [aws_eks_node_group.this]
  filter {
    name   = "tag:eks:nodegroup-name"
    values = [aws_eks_node_group.this.node_group_name]
  }
}

data "aws_instance" "eks_node_detail" {
  depends_on  = [data.aws_instances.eks_node]
  instance_id = data.aws_instances.eks_node.ids[0]
}

resource "null_resource" "validator" {
  depends_on = [
    aws_eks_node_group.this,
    helm_release.aws_observability,
    null_resource.update_image,
    kubernetes_deployment.lis_csi_io_workload,
  ]

  triggers = {
    always_run = timestamp()
  }

  provisioner "local-exec" {
    command = <<-EOT
      echo "Validating CloudWatch Agent with LIS CSI NVME instance store metrics"
      cd ../../../..
      go test ${var.test_dir} -timeout 1h \
      -eksClusterName=${aws_eks_cluster.this.name} \
      -computeType=EKS -v \
      -eksDeploymentStrategy=DAEMON \
      -instanceId=${data.aws_instance.eks_node_detail.instance_id}
    EOT
  }
}
