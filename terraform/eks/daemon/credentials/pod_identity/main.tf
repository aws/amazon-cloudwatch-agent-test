// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "common" {
  source = "../../../../common"
}

module "basic_components" {
  source = "../../../../basic_components"

  region = var.region
}

locals {
  aws_eks = "aws eks --region ${var.region}"
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

# EKS Node Groups
resource "aws_eks_node_group" "this" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "cwagent-eks-integ-node-${module.common.testing_id}"
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
    aws_iam_role_policy_attachment.pod_CloudWatchAgentServerPolicy
  ]
}

resource "aws_eks_addon" "pod_identity_addon" {
  cluster_name = aws_eks_cluster.this.name
  addon_name   = "eks-pod-identity-agent"
  depends_on   = [aws_eks_node_group.this]
}

# EKS Node IAM Role
resource "aws_iam_role" "node_role" {
  name = "cwagent-eks-Worker-Role-${module.common.testing_id}"

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

data "aws_iam_policy_document" "pod-identity-policy" {
  statement {
    effect = "Allow"

    principals {
      type        = "Service"
      identifiers = ["pods.eks.amazonaws.com"]
    }

    actions = [
      "sts:AssumeRole",
      "sts:TagSession"
    ]
  }
}


resource "aws_iam_role" "pod-identity-role" {
  name               = "cwagent-eks-pod-identity-role-${module.common.testing_id}"
  assume_role_policy = data.aws_iam_policy_document.pod-identity-policy.json
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

resource "aws_iam_role_policy_attachment" "pod_CloudWatchAgentServerPolicy" {
  policy_arn = "arn:aws:iam::aws:policy/CloudWatchAgentServerPolicy"
  role       = aws_iam_role.pod-identity-role.name
}

resource "aws_eks_pod_identity_association" "association" {
  cluster_name    = aws_eks_cluster.this.name
  namespace       = "amazon-cloudwatch"
  service_account = "cloudwatch-agent"
  role_arn        = aws_iam_role.pod-identity-role.arn
  depends_on      = [aws_eks_cluster.this]
}

# TODO: these security groups be created once and then reused
# EKS Cluster Security Group
resource "aws_security_group" "eks_cluster_sg" {
  name        = "cwagent-eks-cluster-sg-${module.common.testing_id}"
  description = "Cluster communication with worker nodes"
  vpc_id      = module.basic_components.vpc_id
}

resource "aws_security_group_rule" "cluster_inbound" {
  description              = "Allow worker nodes to communicate with the cluster API Server"
  from_port                = 443
  protocol                 = "tcp"
  security_group_id        = aws_security_group.eks_cluster_sg.id
  source_security_group_id = aws_security_group.eks_nodes_sg.id
  to_port                  = 443
  type                     = "ingress"
}

resource "aws_security_group_rule" "cluster_outbound" {
  description              = "Allow cluster API Server to communicate with the worker nodes"
  from_port                = 1024
  protocol                 = "tcp"
  security_group_id        = aws_security_group.eks_cluster_sg.id
  source_security_group_id = aws_security_group.eks_nodes_sg.id
  to_port                  = 65535
  type                     = "egress"
}


# EKS Node Security Group
resource "aws_security_group" "eks_nodes_sg" {
  name        = "cwagent-eks-node-sg-${module.common.testing_id}"
  description = "Security group for all nodes in the cluster"
  vpc_id      = module.basic_components.vpc_id

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group_rule" "nodes_internal" {
  description              = "Allow nodes to communicate with each other"
  from_port                = 0
  protocol                 = "-1"
  security_group_id        = aws_security_group.eks_nodes_sg.id
  source_security_group_id = aws_security_group.eks_nodes_sg.id
  to_port                  = 65535
  type                     = "ingress"
}

resource "aws_security_group_rule" "nodes_cluster_inbound" {
  description              = "Allow worker Kubelets and pods to receive communication from the cluster control plane"
  from_port                = 1025
  protocol                 = "tcp"
  security_group_id        = aws_security_group.eks_nodes_sg.id
  source_security_group_id = aws_security_group.eks_cluster_sg.id
  to_port                  = 65535
  type                     = "ingress"
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
    aws_eks_addon.pod_identity_addon,
    aws_eks_pod_identity_association.association,
    data.external.clone_helm_chart,
  ]
}

resource "null_resource" "kubectl" {
  depends_on = [
    aws_eks_cluster.this,
    aws_eks_node_group.this,
  ]
  provisioner "local-exec" {
    command = <<-EOT
      ${local.aws_eks} update-kubeconfig --name ${aws_eks_cluster.this.name}
      ${local.aws_eks} list-clusters --output text
      ${local.aws_eks} describe-cluster --name ${aws_eks_cluster.this.name} --output text
    EOT
  }
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

resource "kubernetes_pod" "log_generator" {
  depends_on = [aws_eks_node_group.this]
  metadata {
    name      = "log-generator"
    namespace = "default"
  }

  spec {
    container {
      name  = "log-generator"
      image = "busybox"

      # Run shell script that generate a log line every second
      command = ["/bin/sh", "-c"]
      args    = ["while true; do echo \"Log entry at $(date)\"; sleep 1; done"]
    }
    restart_policy = "Always"
  }
}

# Storage Class for PV testing (using hostPath for simplicity)
resource "kubernetes_storage_class" "test_storage_class" {
  depends_on = [helm_release.aws_observability]
  metadata {
    name = "test-storage-class-${module.common.testing_id}"
  }
  storage_provisioner = "kubernetes.io/no-provisioner"
  volume_binding_mode = "WaitForFirstConsumer"
}

# PersistentVolume
resource "kubernetes_persistent_volume" "test_pv" {
  depends_on = [kubernetes_storage_class.test_storage_class]
  metadata {
    name = "test-pv-${module.common.testing_id}"
  }
  spec {
    capacity = {
      storage = "1Gi"
    }
    access_modes       = ["ReadWriteOnce"]
    storage_class_name = kubernetes_storage_class.test_storage_class.metadata[0].name
    persistent_volume_source {
      host_path {
        path = "/tmp/test-pv"
      }
    }
  }
}

# PersistentVolumeClaim
resource "kubernetes_persistent_volume_claim" "test_pvc" {
  depends_on = [
    helm_release.aws_observability,
    kubernetes_persistent_volume.test_pv
  ]
  metadata {
    name      = "test-pvc-${module.common.testing_id}"
    namespace = "amazon-cloudwatch"
  }
  spec {
    access_modes       = ["ReadWriteOnce"]
    storage_class_name = kubernetes_storage_class.test_storage_class.metadata[0].name
    volume_name        = kubernetes_persistent_volume.test_pv.metadata[0].name
    resources {
      requests = {
        storage = "1Gi"
      }
    }
  }
}

# Pod that uses the PVC (to trigger bound status and generate metrics)
resource "kubernetes_pod" "test_pod_with_pvc" {
  depends_on = [kubernetes_persistent_volume_claim.test_pvc]
  metadata {
    name      = "test-pod-with-pvc-${module.common.testing_id}"
    namespace = "amazon-cloudwatch"
    labels = {
      app = "pv-test"
    }
  }
  spec {
    termination_grace_period_seconds = 0
    container {
      name    = "test-container"
      image   = "busybox"
      command = ["sleep", "3600"]
      volume_mount {
        name       = "test-volume"
        mount_path = "/data"
      }
    }
    volume {
      name = "test-volume"
      persistent_volume_claim {
        claim_name = kubernetes_persistent_volume_claim.test_pvc.metadata[0].name
      }
    }
    restart_policy = "Always"
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
    aws_eks_addon.pod_identity_addon,
    helm_release.aws_observability,
    null_resource.update_image,
    kubernetes_pod.log_generator,
    kubernetes_pod.test_pod_with_pvc,
  ]

  triggers = {
    always_run = timestamp()
  }

  provisioner "local-exec" {
    command = <<-EOT
      echo "Validating CloudWatch Agent with pod identity credential"
      cd ../../../../..
      go test ./test/metric_value_benchmark -timeout 1h -eksClusterName=${aws_eks_cluster.this.name} -computeType=EKS -v -eksDeploymentStrategy=PODIDENTITY -instanceId=${data.aws_instance.eks_node_detail.instance_id} &&
      go test ./test/fluent -eksClusterName=${aws_eks_cluster.this.name} -computeType=EKS -v -eksDeploymentStrategy=DAEMON
    EOT
  }
}

