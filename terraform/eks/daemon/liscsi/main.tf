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

# EKS Node Group
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

# EKS Node IAM Role
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

resource "null_resource" "kubectl" {
  depends_on = [
    aws_eks_cluster.this,
    aws_eks_node_group.this
  ]
  provisioner "local-exec" {
    command = <<-EOT
      ${local.aws_eks} update-kubeconfig --name ${aws_eks_cluster.this.name}
      ${local.aws_eks} list-clusters --output text
      ${local.aws_eks} describe-cluster --name ${aws_eks_cluster.this.name} --output text
    EOT
  }
}

# LIS CSI addon
resource "aws_eks_addon" "lis_csi_addon" {
  depends_on           = [aws_eks_node_group.this]
  cluster_name         = aws_eks_cluster.this.name
  addon_name           = "aws-ec2-local-instance-store-csi-driver"
  configuration_values = jsonencode({ metrics = { enabled = true } })
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
  depends_on = [helm_release.aws_observability, null_resource.kubectl]
  triggers = {
    timestamp = "${timestamp()}"
  }
  provisioner "local-exec" {
    command = <<-EOT
      echo "Patching CWAgent image to ${var.cwagent_image_repo}:${var.cwagent_image_tag}"
      kubectl -n amazon-cloudwatch patch AmazonCloudWatchAgent cloudwatch-agent --type='json' -p='[{"op": "replace", "path": "/spec/image", "value": "${var.cwagent_image_repo}:${var.cwagent_image_tag}"}]'
      echo "Waiting for CWAgent DaemonSet rollout..."
      sleep 10
      kubectl rollout status daemonset/cloudwatch-agent -n amazon-cloudwatch --timeout=300s
      echo "CWAgent pods after rollout:"
      kubectl get pods -n amazon-cloudwatch -l app.kubernetes.io/name=cloudwatch-agent -o wide
      echo "CWAgent image in use:"
      kubectl get pods -n amazon-cloudwatch -l app.kubernetes.io/name=cloudwatch-agent -o jsonpath='{.items[*].spec.containers[*].image}'
      echo ""
    EOT
  }
}

resource "null_resource" "wait_for_lis_csi" {
  depends_on = [aws_eks_addon.lis_csi_addon, null_resource.kubectl]
  provisioner "local-exec" {
    command = <<-EOT
      echo "Waiting for LIS CSI DaemonSet rollout..."
      kubectl rollout status daemonset/ec2-instance-store-plugin -n kube-system --timeout=300s
      echo "LIS CSI pods:"
      kubectl get pods -n kube-system -l app.kubernetes.io/name=ec2-instance-store-plugin -o wide
      echo "StorageClasses:"
      kubectl get sc
      echo "NVMe devices:"
      kubectl get nvmedevices 2>/dev/null || echo "nvmedevices CRD not found"
    EOT
  }
}

resource "kubernetes_deployment_v1" "lis_csi_io_workload" {
  depends_on = [null_resource.wait_for_lis_csi]
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
          ephemeral {
            volume_claim_template {
              spec {
                access_modes       = ["ReadWriteOnce"]
                storage_class_name = "ec2-instance-store-sc"
                resources {
                  requests = {
                    storage = "1Gi"
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}

# Get the single instance ID of the node in the node group
data "aws_instances" "eks_node" {
  depends_on = [
    aws_eks_node_group.this
  ]
  filter {
    name   = "tag:eks:nodegroup-name"
    values = [aws_eks_node_group.this.node_group_name]
  }
}

# Retrieve details of the single instance to get private DNS
data "aws_instance" "eks_node_detail" {
  depends_on = [
    data.aws_instances.eks_node
  ]
  instance_id = data.aws_instances.eks_node.ids[0]
}

resource "null_resource" "validator" {
  depends_on = [
    aws_eks_node_group.this,
    aws_eks_addon.lis_csi_addon,
    helm_release.aws_observability,
    null_resource.update_image,
    kubernetes_deployment_v1.lis_csi_io_workload,
  ]

  triggers = {
    always_run = timestamp()
  }

  provisioner "local-exec" {
    command = <<-EOT
      echo "Validating CloudWatch Agent with LIS CSI NVME instance store metrics"
      echo "=== CWAgent pods ==="
      kubectl get pods -n amazon-cloudwatch -o wide
      echo "=== CWAgent image ==="
      kubectl get pods -n amazon-cloudwatch -l app.kubernetes.io/name=cloudwatch-agent -o jsonpath='{.items[*].spec.containers[*].image}'
      echo ""
      echo "=== LIS CSI pods ==="
      kubectl get pods -n kube-system -l app.kubernetes.io/name=ec2-instance-store-plugin -o wide
      echo "=== IO workload pods ==="
      kubectl get pods -n default -l app=liscsi-integ-test -o wide
      echo "=== PVCs ==="
      kubectl get pvc -A
      echo "=== LIS CSI metrics endpoint ==="
      kubectl get svc -n kube-system -l app.kubernetes.io/name=ec2-instance-store-plugin 2>/dev/null || echo "No LIS CSI service found"
      echo "=== Running go test ==="
      cd ../../../..
      go test ${var.test_dir} -timeout 1h -eksClusterName=${aws_eks_cluster.this.name} -computeType=EKS -v -eksDeploymentStrategy=DAEMON -instanceId=${data.aws_instance.eks_node_detail.instance_id}
    EOT
  }
}
