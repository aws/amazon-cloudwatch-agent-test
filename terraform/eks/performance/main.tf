module "common" {
  source = "../../common"
}

module "basic_components" {
  source = "../../basic_components"
}

locals {
  cluster_name = var.cluster_name != "" ? var.cluster_name : "cwagent-eks-performance"
}

# EKS Cluster Creation
resource "aws_eks_cluster" "this" {
  name     = local.cluster_name
  role_arn = module.basic_components.role_arn
  version  = var.k8s_version
  vpc_config {
    subnet_ids         = module.basic_components.public_subnet_ids
    security_group_ids = [module.basic_components.security_group]
  }
}

# EKS Cluster Node Groups
resource "aws_eks_node_group" "this" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "${local.cluster_name}-node"
  node_role_arn   = aws_iam_role.node_role.arn
  subnet_ids      = module.basic_components.public_subnet_ids

  scaling_config {
    desired_size = var.nodes
    max_size     = 500
    min_size     = 1
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

# EKS Cluster Security Groups
resource "aws_security_group_rule" "nodeport_inbound" {
  type              = "ingress"
  from_port         = 30080
  to_port           = 30080
  protocol          = "tcp"
  cidr_blocks       = ["0.0.0.0/0"]
  security_group_id = aws_eks_cluster.this.vpc_config[0].cluster_security_group_id
}

# EKS Node IAM Role
resource "aws_iam_role" "node_role" {
  name = "cwagent-eks-Worker-Role-${module.common.testing_id}"
  assume_role_policy = jsonencode({
    Version = "2012-10-17",
    Statement = [
      {
        Effect = "Allow",
        Principal = {
          Service = "ec2.amazonaws.com"
        },
        Action = "sts:AssumeRole"
      }
    ]
  })
}

# EKS Node Policies
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

# EKS Cluster Auth
data "aws_eks_cluster_auth" "this" {
  name = aws_eks_cluster.this.name
}

# Helm Provider Definition
provider "helm" {
  kubernetes = {
    host                   = aws_eks_cluster.this.endpoint
    cluster_ca_certificate = base64decode(aws_eks_cluster.this.certificate_authority[0].data)
    token                  = data.aws_eks_cluster_auth.this.token
  }
}

# Conditional resource for helm charts
resource "null_resource" "helm_charts" {

  provisioner "local-exec" {
    command = <<-EOT
      set -e
      echo "Cloning helm-charts repository..."
      git clone https://github.com/aws-observability/helm-charts.git ${path.module}/helm-charts
      cd ${path.module}/helm-charts
      echo "Checking out branch: ${var.helm_charts_branch}"
      git checkout ${var.helm_charts_branch}
      echo "Contents of charts directory:"
      ls -R ${path.module}/helm-charts/charts
      if [ ! -d "${path.module}/helm-charts/charts/amazon-cloudwatch-observability" ]; then
        echo "Error: amazon-cloudwatch-observability chart not found!"
        exit 1
      fi
    EOT
  }

  provisioner "local-exec" {
    when    = destroy
    command = "rm -rf ${path.module}/helm-charts"
  }
}

# Install Helm chart
resource "helm_release" "cloudwatch_observability" {
  depends_on = [null_resource.helm_charts]

  name             = "amazon-cloudwatch-observability"
  chart            = "${path.module}/helm-charts/charts/amazon-cloudwatch-observability"
  namespace        = "amazon-cloudwatch"
  create_namespace = true

  set = [
    {
      name  = "clusterName"
      value = local.cluster_name
    },
    {
      name  = "region"
      value = var.region
    },
    {
      name  = "agent.image.repository"
      value = var.cloudwatch_agent_repository
    },
    {
      name  = "agent.image.tag"
      value = var.cloudwatch_agent_tag
    },
    {
      name  = "agent.image.repositoryDomainMap.public"
      value = var.cloudwatch_agent_repository_url
    },
    {
      name  = "manager.image.repository"
      value = var.cloudwatch_agent_operator_repository
    },
    {
      name  = "manager.image.tag"
      value = var.cloudwatch_agent_operator_tag
    },
    {
      name  = "manager.image.repositoryDomainMap.public"
      value = var.cloudwatch_agent_operator_repository_url
    }
  ]
}

resource "null_resource" "cluster_manager" {
  depends_on = [
    aws_eks_cluster.this,
    aws_eks_node_group.this,
    null_resource.helm_charts,
  ]

  triggers = {
    cluster_name            = aws_eks_cluster.this.name
    region                  = var.region
    test_dir                = var.test_dir
    eks_deployment_strategy = var.eks_deployment_strategy
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

#       echo "=== Configuration Files ==="
#       echo "Agent Config: ${var.test_dir}/${var.agent_config}"
#       echo "OpenTelemetry Config: ${var.otel_config != "" ? "${var.test_dir}/${var.otel_config}" : "Not specified"}"
#       echo "Prometheus Config: ${var.prometheus_config != "" ? "${var.test_dir}/${var.prometheus_config}" : "Not specified"}"
#       echo "Sample App: ${var.test_dir}/${var.sample_app}"
    EOT
  }

#   provisioner "local-exec" {
#     when    = destroy
#     command = <<-EOT
#       echo "=== Starting Cleanup Process ==="
#       echo "=== Cleanup Configuration ==="
#       echo "Region: ${self.triggers.region}"
#       echo "EKS Cluster Name: ${self.triggers.cluster_name}"
#       echo "Compute Type: EKS"
#       echo "EKS Deployment Strategy: ${self.triggers.eks_deployment_strategy}"
#     EOT
#   }
}

output "cluster_name" {
  value = aws_eks_cluster.this.name
}