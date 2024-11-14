// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "common" {
  source = "../../common"
}

module "basic_components" {
  source = "../../basic_components"
}

locals {
  aws_eks      = "aws eks --region ${var.region}"
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
    desired_size = 1
    max_size     = 1
    min_size     = 1
  }

  ami_type       = "AL2_x86_64"
  capacity_type  = "ON_DEMAND"
  disk_size      = 20
  instance_types = ["t3a.medium"]

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

resource "null_resource" "install_helm_release" {
  depends_on = [
    null_resource.kubectl,
    aws_eks_cluster.this
  ]

  provisioner "local-exec" {
    command = <<-EOT
      helm upgrade --install amazon-cloudwatch-observability \
        ${path.module}/helm-charts/charts/amazon-cloudwatch-observability \
        --values ${path.module}/helm-charts/charts/amazon-cloudwatch-observability/values.yaml \
        --set clusterName=${aws_eks_cluster.this.name} \
        --set region=${var.region} \
        --set agent.image.repository=${var.cloudwatch_agent_repository} \
        --set agent.image.tag=${var.cloudwatch_agent_tag} \
        --set agent.image.repositoryDomainMap.public=${var.cloudwatch_agent_repository_url} \
        --set manager.image.repository=${var.cloudwatch_agent_operator_repository} \
        --set manager.image.tag=${var.cloudwatch_agent_operator_tag} \
        --set manager.image.repositoryDomainMap.public=${var.cloudwatch_agent_operator_repository_url} \
        --namespace amazon-cloudwatch \
        --create-namespace \
        ${var.agent-config != "" ? "--set-json agent.config='${jsonencode(jsondecode(file(var.agent-config)))}'" : ""} \
        ${var.otel-config != "" ? "--set-file agent.otelConfig=${yamlencode(yamldecode(file(var.otel-config)))}" : ""} \
        ${var.prometheus-config != "" ? "--set-file agent.prometheus.config=${yamlencode(yamldecode(file(var.prometheus-config)))}" : ""}
    EOT

    environment = {
      KUBECONFIG = "$HOME/.kube/config"
    }
  }
}

resource "null_resource" "test" {
  depends_on = [
    null_resource.install_helm_release
  ]

  provisioner "local-exec" {
    command = "go test -v ${var.validate_test}"
  }
}
