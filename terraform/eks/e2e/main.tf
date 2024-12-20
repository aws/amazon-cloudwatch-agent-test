// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "common" {
  source = "../../common"
}

module "basic_components" {
  source = "../../basic_components"
}

locals {
  cluster_name = var.cluster_name != "" ? var.cluster_name : "cwagent-monitoring-config-e2e-eks"
}

data "aws_eks_cluster_auth" "this" {
  name = aws_eks_cluster.this.name
}

resource "aws_eks_cluster" "this" {
  name     = "${local.cluster_name}-${module.common.testing_id}"
  role_arn = module.basic_components.role_arn
  version  = var.k8s_version
  vpc_config {
    subnet_ids         = module.basic_components.public_subnet_ids
    security_group_ids = [module.basic_components.security_group]
  }
}

resource "aws_eks_node_group" "this" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "${local.cluster_name}-node"
  node_role_arn   = aws_iam_role.node_role.arn
  subnet_ids      = module.basic_components.public_subnet_ids

  scaling_config {
    desired_size = var.nodes
    max_size     = var.nodes
    min_size     = var.nodes
  }

  ami_type       = var.ami_type
  capacity_type  = "ON_DEMAND"
  disk_size      = 20
  instance_types = [var.instance_type]

  depends_on = [
    aws_iam_role_policy_attachment.node_CloudWatchAgentServerPolicy,
    aws_iam_role_policy_attachment.node_AmazonEC2ContainerRegistryReadOnly,
    aws_iam_role_policy_attachment.node_AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy
  ]
}

resource "aws_iam_role" "node_role" {
  name = "${local.cluster_name}-Worker-Role-${module.common.testing_id}"

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

resource "null_resource" "helm_charts" {
  provisioner "local-exec" {
    command = <<-EOT
      git clone https://github.com/aws-observability/helm-charts.git helm-charts
      cd helm-charts
      git checkout ${var.helm_charts_branch}
    EOT
  }

  provisioner "local-exec" {
    when    = destroy
    command = "rm -rf ${path.module}/helm-charts"
  }
}

resource "null_resource" "validator" {
  depends_on = [aws_eks_cluster.this, aws_eks_node_group.this, null_resource.helm_charts]

  triggers = {
    cluster_name = aws_eks_cluster.this.name
    region       = var.region
    test_dir     = var.test_dir
  }

  provisioner "local-exec" {
    command = <<-EOT
      echo "Validating K8s resources and metrics"
      go test -timeout 2h -v ${var.test_dir} \
      -region=${var.region} \
      -k8s_version=${var.k8s_version} \
      -eksClusterName=${aws_eks_cluster.this.name} \
      -computeType=EKS \
      -eksDeploymentStrategy=DAEMON \
      -helm_charts_branch=${var.helm_charts_branch} \
      -cloudwatch_agent_repository=${var.cloudwatch_agent_repository} \
      -cloudwatch_agent_tag=${var.cloudwatch_agent_tag} \
      -cloudwatch_agent_repository_url=${var.cloudwatch_agent_repository_url} \
      -cloudwatch_agent_operator_repository=${var.cloudwatch_agent_operator_repository} \
      -cloudwatch_agent_operator_tag=${var.cloudwatch_agent_operator_tag} \
      -cloudwatch_agent_operator_repository_url=${var.cloudwatch_agent_operator_repository_url} \
      -cloudwatch_agent_target_allocator_repository=${var.cloudwatch_agent_target_allocator_repository} \
      -cloudwatch_agent_target_allocator_tag=${var.cloudwatch_agent_target_allocator_tag} \
      -cloudwatch_agent_target_allocator_repository_url=${var.cloudwatch_agent_target_allocator_repository_url} \
      -agent_config="${var.test_dir}/${var.agent_config}" \
      ${var.otel_config != "" ? "-otel_config=\"${var.test_dir}/${var.otel_config}\"" : ""} \
      ${var.prometheus_config != "" ? "-prometheus_config=\"${var.test_dir}/${var.prometheus_config}\"" : ""} \
      -sample_app="${var.test_dir}/${var.sample_app}"
    EOT
  }

  provisioner "local-exec" {
    when    = destroy
    command = <<-EOT
      echo "Running cleanup for K8s resources"
      go test -timeout 30m -v ${self.triggers.test_dir} \
      -destroy=true \
      -region=${self.triggers.region} \
      -eksClusterName=${self.triggers.cluster_name} \
      -computeType=EKS \
      -eksDeploymentStrategy=DAEMON
    EOT
  }
}
