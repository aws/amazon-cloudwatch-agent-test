// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "common" {
  source = "../../../common"
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

# EKS Node Groups
resource "aws_eks_node_group" "this" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "cwagent-addon-eks-integ-node-${module.common.testing_id}"
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

resource "aws_iam_role_policy_attachment" "node_AmazonEBSCSIDriverPolicy" {
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonEBSCSIDriverPolicy"
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

resource "aws_eks_addon" "ebs_csi_addon" {
  depends_on   = [aws_eks_node_group.this]
  cluster_name = aws_eks_cluster.this.name
  addon_name   = "aws-ebs-csi-driver"
}

resource "null_resource" "clone_helm_chart" {
  triggers = {
    timestamp = "${timestamp()}" # Forces re-run on every apply
  }
  provisioner "local-exec" {
    command = <<-EOT
      if [ ! -d "./helm-charts" ]; then
        git clone -b ${var.helm_chart_branch} https://github.com/aws-observability/helm-charts.git ./helm-charts
      fi
    EOT
  }
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
    value = "us-west-2"
  }
  depends_on = [
    aws_eks_cluster.this,
    aws_eks_node_group.this,
    null_resource.clone_helm_chart,
  ]
}

resource "null_resource" "update_image" {
  depends_on = [helm_release.aws_observability, null_resource.kubectl]
  triggers = {
    timestamp = "${timestamp()}" # Forces re-run on every apply
  }
  provisioner "local-exec" {
    command = <<-EOT
      kubectl -n amazon-cloudwatch patch AmazonCloudWatchAgent cloudwatch-agent --type='json' -p='[{"op": "replace", "path": "/spec/image", "value": "${var.cwagent_image_repo}:${var.cwagent_image_tag}"}]'
      kubectl set image deployment/amazon-cloudwatch-observability-controller-manager -n amazon-cloudwatch manager=public.ecr.aws/cloudwatch-agent/cloudwatch-agent-operator:latest
      sleep 10
    EOT
  }
}

resource "kubernetes_storage_class" "ebs_sc" {
  depends_on = [aws_eks_addon.ebs_csi_addon]
  metadata {
    name = "ebs-sc-${module.common.testing_id}"
  }
  
  storage_provisioner = "ebs.csi.aws.com"
  volume_binding_mode = "WaitForFirstConsumer"
  
  parameters = {
    type      = "gp3"
    fsType    = "ext4"
    encrypted = "true"
  }
}
resource "kubernetes_persistent_volume_claim" "ebs_pvc" {
  depends_on = [kubernetes_storage_class.ebs_sc]
  metadata {
    name      = "ebs-pvc-${module.common.testing_id}"
    namespace = "default"
  }

  wait_until_bound = false
  
  spec {
    access_modes = ["ReadWriteOnce"]
    storage_class_name = kubernetes_storage_class.ebs_sc.metadata[0].name
    
    resources {
      requests = {
        storage = "5Gi"
      }
    }
  }
}

resource "kubernetes_deployment" "ebs_deployment" {
  depends_on = [kubernetes_persistent_volume_claim.ebs_pvc]
  metadata {
    name = "app"
  }
  
  spec {
    replicas = 1
    
    selector {
      match_labels = {
        app = "app"
      }
    }
    
    template {
      metadata {
        labels = {
          app = "app"
        }
      }
      
      spec {
        container {
          name  = "app"
          image = "public.ecr.aws/amazonlinux/amazonlinux"
          command = ["/bin/bash", "-c", "sleep infinity"]
          
          volume_mount {
            name       = "persistent-storage"
            mount_path = "/data"
          }
        }
        
        volume {
          name = "persistent-storage"
          persistent_volume_claim {
            claim_name = kubernetes_persistent_volume_claim.ebs_pvc.metadata[0].name
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
    aws_eks_addon.ebs_csi_addon,
    helm_release.aws_observability,
    null_resource.update_image,
    kubernetes_deployment.ebs_deployment,
  ]

  triggers = {
    always_run = timestamp()
  }

  provisioner "local-exec" {
    command = <<-EOT
      echo "Validating CloudWatch Agent with EBS CSI NVMe metrics"
      cd ../../../../..
      go test ${var.test_dir} -timeout 1h -eksClusterName=${aws_eks_cluster.this.name} -computeType=EKS -v -eksDeploymentStrategy=DAEMON -instanceId=${data.aws_instance.eks_node_detail.instance_id}
    EOT
  }
}

