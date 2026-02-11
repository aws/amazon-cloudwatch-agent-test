// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "common" {
  source = "../../../common"
}

module "basic_components" {
  source = "../../../basic_components"
  region = var.region
}


data "aws_eks_cluster_auth" "this" {
  name = aws_eks_cluster.this.name
}

locals {
  role_arn = format("%s%s", module.basic_components.role_arn, var.beta ? "-eks-beta" : "")
  aws_eks  = format("%s%s", "aws eks --region ${var.region}", var.beta ? " --endpoint ${var.beta_endpoint}" : "")
}

resource "aws_eks_cluster" "this" {
  name     = "cwagent-addon-eks-integ-${module.common.testing_id}"
  role_arn = local.role_arn
  version  = var.k8s_version
  enabled_cluster_log_types = [
    "api",
    "audit",
    "authenticator",
    "controllerManager",
    "scheduler"
  ]
  vpc_config {
    subnet_ids         = module.basic_components.public_subnet_ids
    security_group_ids = [module.basic_components.security_group]
  }
}

# EKS Node Groups
resource "aws_eks_node_group" "this" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "cwagent-addon-eks-integ-node"
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
  name = "cwagent-addon-eks-Worker-Role-${module.common.testing_id}"

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

# Amazon CloudWatch Namespace
resource "kubernetes_namespace" "namespace" {
  metadata {
    name = "amazon-cloudwatch"
  }
}

# NVIDIA Device Plugin DaemonSet
resource "kubernetes_daemonset" "nvidia_device_plugin" {
  depends_on = [
    aws_eks_cluster.this,
    aws_eks_node_group.this,
  ]
  metadata {
    name      = "nvidia-device-plugin-daemonset"
    namespace = "kube-system"
  }
  spec {
    selector {
      match_labels = {
        name = "nvidia-device-plugin-ds"
      }
    }
    template {
      metadata {
        labels = {
          name = "nvidia-device-plugin-ds"
        }
      }
      spec {
        toleration {
          key      = "nvidia.com/gpu"
          operator = "Exists"
          effect   = "NoSchedule"
        }
        priority_class_name = "system-node-critical"
        container {
          name  = "nvidia-device-plugin-ctr"
          image = "nvcr.io/nvidia/k8s-device-plugin:v0.17.0"
          args  = ["--fail-on-init-error=false"]
          security_context {
            allow_privilege_escalation = false
            capabilities {
              drop = ["ALL"]
            }
          }
          volume_mount {
            name       = "device-plugin"
            mount_path = "/var/lib/kubelet/device-plugins"
          }
        }
        volume {
          name = "device-plugin"
          host_path {
            path = "/var/lib/kubelet/device-plugins"
          }
        }
      }
    }
  }
}

# Wait for NVIDIA device plugin to be truly ready
resource "null_resource" "wait_for_nvidia_ready" {
  depends_on = [
    null_resource.kubectl,
    kubernetes_daemonset.nvidia_device_plugin
  ]
  provisioner "local-exec" {
    command = <<-EOT
      if ! kubectl wait --for=condition=ready pod -l name=nvidia-device-plugin-ds -n kube-system --timeout=300s; then
        echo "ERROR: NVIDIA device plugin failed to become ready"
        kubectl get pods -n kube-system -l name=nvidia-device-plugin-ds
        kubectl describe pods -n kube-system -l name=nvidia-device-plugin-ds
        exit 1
      fi
      
      echo "Waiting for GPU resources to be registered with kubelet..."
      for i in $(seq 1 30); do
        if kubectl get nodes -o jsonpath='{.items[*].status.allocatable.nvidia\.com/gpu}' | grep -q "1"; then
          echo "GPU resources found on node"
          exit 0
        fi
        echo "Attempt $i/30: GPU not yet available, waiting 10s..."
        sleep 10
      done
      
      echo "ERROR: No GPU resources found on nodes after 5 minutes"
      kubectl get nodes -o json | jq '.items[].status.allocatable'
      exit 1
    EOT
  }
}

# GPU Burner Deployment - Real GPU workload for testing
resource "kubernetes_deployment" "gpu_burner" {
  depends_on = [
    kubernetes_namespace.namespace,
    aws_eks_cluster.this,
    aws_eks_node_group.this,
    null_resource.wait_for_nvidia_ready,
  ]
  metadata {
    name      = "gpu-burn"
    namespace = "amazon-cloudwatch"
    labels = {
      app = "gpu-burn"
    }
  }
  spec {
    replicas = 1
    selector {
      match_labels = {
        app = "gpu-burn"
      }
    }
    template {
      metadata {
        labels = {
          app = "gpu-burn"
        }
      }
      spec {
        container {
          name              = "main"
          image             = "oguzpastirmaci/gpu-burn"
          image_pull_policy = "IfNotPresent"
          command = [
            "bash",
            "-c",
            "while true; do /app/gpu_burn 20; sleep 20; done"
          ]
          resources {
            limits = {
              "nvidia.com/gpu" = "1"
            }
          }
        }
      }
    }
  }
}

# GPU Burner Service
resource "kubernetes_service" "gpu_burner_service" {
  depends_on = [
    kubernetes_namespace.namespace,
    kubernetes_deployment.gpu_burner,
  ]
  metadata {
    name      = "gpu-burn-service"
    namespace = "amazon-cloudwatch"
    labels = {
      app = "gpu-burn"
    }
  }
  spec {
    type = "ClusterIP"
    selector = {
      app = "gpu-burn"
    }
    port {
      port        = 80
      target_port = 80
      protocol    = "TCP"
    }
  }
}

resource "aws_eks_addon" "this" {
  depends_on = [
    null_resource.kubectl,
    kubernetes_daemonset.nvidia_device_plugin,
    kubernetes_deployment.gpu_burner,
    kubernetes_service.gpu_burner_service,
  ]
  addon_name   = var.addon_name
  cluster_name = aws_eks_cluster.this.name
}

# Patch CloudWatch Agent image for testing
resource "null_resource" "patch_agent_image" {
  depends_on = [
    null_resource.kubectl, # Kubeconfig already set up
    aws_eks_addon.this,    # Add-on deployed
  ]

  provisioner "local-exec" {
    command = <<-EOT
      echo "Waiting for CloudWatch Agent DaemonSet to be ready before patching..."
      kubectl rollout status daemonset/cloudwatch-agent -n amazon-cloudwatch --timeout=300s
      
      echo "Patching CloudWatch Agent image to ${var.cwagent_image_repo}:${var.cwagent_image_tag}..."
      kubectl patch amazoncloudwatchagents -n amazon-cloudwatch cloudwatch-agent \
        --type='json' \
        -p='[{"op": "replace", "path": "/spec/image", "value": "${var.cwagent_image_repo}:${var.cwagent_image_tag}"}]'
      
      echo "Waiting for CloudWatch Agent DaemonSet rollout after patching..."
      kubectl rollout status daemonset/cloudwatch-agent -n amazon-cloudwatch --timeout=300s
      
      echo "CloudWatch Agent image patched and rolled out successfully"
    EOT
  }

  # Re-run if image changes
  triggers = {
    image_repo = var.cwagent_image_repo
    image_tag  = var.cwagent_image_tag
  }
}

# Run Go tests after infrastructure and agent patching is complete
resource "null_resource" "validator" {
  depends_on = [
    aws_eks_cluster.this,
    aws_eks_node_group.this,
    aws_eks_addon.this,
    kubernetes_daemonset.nvidia_device_plugin,
    kubernetes_deployment.gpu_burner,
    kubernetes_service.gpu_burner_service,
    null_resource.patch_agent_image,
  ]

  provisioner "local-exec" {
    command = <<-EOT
      cd ../../../..
      i=0
      while [ $i -lt 10 ]; do
        i=$((i+1))
        go test ${var.test_dir} -eksClusterName=${aws_eks_cluster.this.name} -computeType=EKS -v -eksDeploymentStrategy=DAEMON -eksGpuType=nvidia && exit 0
        sleep 60
      done
      exit 1
    EOT
  }
}

output "eks_cluster_name" {
  value = aws_eks_cluster.this.name
}
