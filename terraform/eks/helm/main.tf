module "common" {
  source = "../../common"
}

module "basic_components" {
  source = "../../basic_components"
}

module "performance" {
  source = "./performance"
}

# Helm Provider Definition
provider "helm" {
  kubernetes {
    host                   = module.performance.cluster_endpoint
    cluster_ca_certificate = base64decode(module.performance.cluster_ca_certificate)
    token                  = module.performance.cluster_auth_token
  }
}

# Install Helm chart
resource "helm_release" "cloudwatch_observability" {
  name             = "amazon-cloudwatch-observability"
  chart            = "${path.module}/helm-charts/charts/amazon-cloudwatch-observability"
  namespace        = "amazon-cloudwatch"
  create_namespace = true

  set = [
    {
      name  = "clusterName"
      value = module.performance.cluster_name
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
  triggers = {
    cluster_name            = module.performance.cluster_name
    region                  = var.region
    test_dir                = var.test_dir
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

    EOT
  }

}

output "cluster_name" {
  value = module.performance.cluster_name
}