module "common" {
  source = "../../common"
}

module "basic_components" {
  source    = "../../basic_components"
  vpc_name  = var.vpc_name
  ip_family = var.ip_family
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
    security_group_ids = var.vpc_name == "" ? [module.basic_components.security_group] : []
  }

  kubernetes_network_config {
    ip_family = var.ip_family
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
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy,
    aws_iam_role_policy_attachment.node_CNI_IPv6_Policy
  ]
}

resource "aws_security_group_rule" "nodeport_inbound" {
  type              = "ingress"
  from_port         = 30080
  to_port           = 30080
  protocol          = "tcp"
  cidr_blocks       = var.ip_family == "ipv4" ? ["0.0.0.0/0"] : []
  ipv6_cidr_blocks  = var.ip_family == "ipv6" ? ["::/0"] : []
  security_group_id = aws_eks_cluster.this.vpc_config[0].cluster_security_group_id
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

resource "aws_iam_policy" "node_CNI_IPv6_Policy" {
  count = var.ip_family == "ipv6" ? 1 : 0

  name        = "AmazonEKS_CNI_IPv6_Policy-${module.common.testing_id}"
  description = "EKS VPC CNI policy for IPv6 prefix delegation"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "ec2:AssignIpv6Addresses",
          "ec2:DescribeInstances",
          "ec2:DescribeTags",
          "ec2:DescribeNetworkInterfaces",
          "ec2:DescribeInstanceTypes"
        ]
        Resource = "*"
      },
      {
        Effect = "Allow"
        Action = [
          "ec2:CreateTags"
        ]
        Resource = [
          "arn:aws:ec2:*:*:network-interface/*"
        ]
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "node_CNI_IPv6_Policy" {
  count = var.ip_family == "ipv6" ? 1 : 0

  policy_arn = aws_iam_policy.node_CNI_IPv6_Policy[0].arn
  role       = aws_iam_role.node_role.name
}

# Conditional resource for helm charts
resource "null_resource" "helm_charts" {
  count = var.eks_installation_type == "HELM_CHART" ? 1 : 0

  provisioner "local-exec" {
    command = <<-EOT
      git clone https://github.com/aws-observability/helm-charts.git ${path.module}/helm-charts
      cd ${path.module}/helm-charts
      git checkout ${var.helm_charts_branch}
    EOT
  }

  provisioner "local-exec" {
    when    = destroy
    command = "rm -rf ${path.module}/helm-charts"
  }
}

# Conditional resource for EKS add-on
resource "aws_eks_addon" "this" {
  count = var.eks_installation_type == "EKS_ADDON" ? 1 : 0

  depends_on = [
    aws_eks_cluster.this,
    aws_eks_node_group.this
  ]
  addon_name           = var.addon_name
  cluster_name         = aws_eks_cluster.this.name
  configuration_values = file("${var.test_dir}/${var.agent_config}")
}

resource "null_resource" "validator" {
  depends_on = [
    aws_eks_cluster.this,
    aws_eks_node_group.this,
    null_resource.helm_charts,
    aws_eks_addon.this
  ]

  triggers = {
    cluster_name            = aws_eks_cluster.this.name
    region                  = var.region
    test_dir                = var.test_dir
    eks_deployment_strategy = var.eks_deployment_strategy
    eks_installation_type   = var.eks_installation_type
  }

  provisioner "local-exec" {
    command = <<-EOT
      echo "=== Starting Validation Process ==="
      echo "=== Configuration Values ==="
      echo "Test Directory: ${var.test_dir}"
      echo "Region: ${var.region}"
      echo "Kubernetes Version: ${var.k8s_version}"
      echo "EKS Cluster Name: ${aws_eks_cluster.this.name}"
      echo "Compute Type: EKS"
      echo "EKS Installation Type: ${var.eks_installation_type}"
      echo "EKS Deployment Strategy: ${var.eks_deployment_strategy}"

      echo "=== CloudWatch Agent Configuration ==="
      echo "Repository: ${var.cloudwatch_agent_repository}"
      echo "Tag: ${var.cloudwatch_agent_tag}"
      echo "Repository URL: ${var.cloudwatch_agent_repository_url}"

      echo "=== Operator Configuration ==="
      echo "Repository: ${var.cloudwatch_agent_operator_repository}"
      echo "Tag: ${var.cloudwatch_agent_operator_tag}"
      echo "Repository URL: ${var.cloudwatch_agent_operator_repository_url}"

      echo "=== Target Allocator Configuration ==="
      echo "Repository: ${var.cloudwatch_agent_target_allocator_repository}"
      echo "Tag: ${var.cloudwatch_agent_target_allocator_tag}"
      echo "Repository URL: ${var.cloudwatch_agent_target_allocator_repository_url}"

      echo "=== Configuration Files ==="
      echo "Agent Config: ${var.test_dir}/${var.agent_config}"
      echo "OpenTelemetry Config: ${var.otel_config != "" ? "${var.test_dir}/${var.otel_config}" : "Not specified"}"
      echo "Prometheus Config: ${var.prometheus_config != "" ? "${var.test_dir}/${var.prometheus_config}" : "Not specified"}"
      echo "Sample App: ${var.test_dir}/${var.sample_app}"

      echo "=== Starting Test Execution ==="
      go test -timeout 2h -v ${var.test_dir} \
      -region=${var.region} \
      -k8s_version=${var.k8s_version} \
      -eksClusterName=${aws_eks_cluster.this.name} \
      -computeType=EKS \
      -eksDeploymentStrategy=${var.eks_deployment_strategy} \
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
      -sample_app="${var.test_dir}/${var.sample_app}" \
      -eks_installation_type=${var.eks_installation_type} \
      -ip_family=${var.ip_family}
    EOT
  }

  provisioner "local-exec" {
    when    = destroy
    command = <<-EOT
      echo "=== Starting Cleanup Process ==="
      echo "=== Cleanup Configuration ==="
      echo "Test Directory: ${self.triggers.test_dir}"
      echo "Region: ${self.triggers.region}"
      echo "EKS Cluster Name: ${self.triggers.cluster_name}"
      echo "Compute Type: EKS"
      echo "EKS Installation Type: ${self.triggers.eks_installation_type}"
      echo "EKS Deployment Strategy: ${self.triggers.eks_deployment_strategy}"

      echo "=== Executing Cleanup ==="
      go test -timeout 30m -v ${self.triggers.test_dir} \
      -destroy \
      -eks_installation_type=${self.triggers.eks_installation_type} \
      -region=${self.triggers.region} \
      -eksClusterName=${self.triggers.cluster_name} \
      -computeType=EKS \
      -eksDeploymentStrategy=${self.triggers.eks_deployment_strategy}

      echo "=== Cleanup Complete ==="
    EOT
  }
}

output "eks_cluster_name" {
  value = aws_eks_cluster.this.name
}