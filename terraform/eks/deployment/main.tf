// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "common" {
  source             = "../../common"
  cwagent_image_repo = var.cwagent_image_repo
  cwagent_image_tag  = var.cwagent_image_tag
}

module "basic_components" {
  source = "../../basic_components"

  region = var.region
}

data "aws_eks_cluster_auth" "this" {
  name = aws_eks_cluster.this.name
}

resource "aws_eks_cluster" "this" {
  name     = "cwagent-eks-integ-${module.common.testing_id}"
  role_arn = module.basic_components.role_arn
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
  node_group_name = "cwagent-eks-integ-node"
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
  instance_types = ["t3.medium"]

  depends_on = [
    aws_iam_role_policy_attachment.node_AmazonEC2ContainerRegistryReadOnly,
    aws_iam_role_policy_attachment.node_AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy,
    aws_iam_role_policy_attachment.node_CloudWatchAgentServerPolicy
  ]
}

# EKS Node IAM Role
#resource "aws_iam_role" "node_role" {
#  name = "cwagent-eks-Worker-Role-${module.common.testing_id}"
#  assume_role_policy = <<POLICY
#{
#  "Version": "2012-10-17",
#  "Statement": [
#    {
#      "Effect": "Allow",
#      "Principal": {
#        "Service": "ec2.amazonaws.com"
#      },
#      "Action": "sts:AssumeRole"
#    }
#  ]
#}
#POLICY
#}

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

resource "kubernetes_namespace" "namespace" {
  metadata {
    name = "amazon-cloudwatch"
    labels = {
      name = "amazon-cloudwatch"
    }
  }
}

# TODO: how do we support different deployment types? Should they be in separate terraform
#       files, and spawn separate tests?
resource "kubernetes_deployment" "service" {
  depends_on = [
    kubernetes_namespace.namespace,
    kubernetes_config_map.cwagentconfig,
    kubernetes_config_map.prometheus_config,
    kubernetes_service_account.cwagentservice,
    kubernetes_service.redis_service,
    aws_eks_node_group.this
  ]
  metadata {
    name      = "cloudwatch-agent"
    namespace = "amazon-cloudwatch"
  }
  spec {
    replicas = 1

    selector {
      match_labels = {
        app = "cloudwatch-agent"
      }
    }
    template {
      metadata {
        labels = {
          app = "cloudwatch-agent"
        }
      }
      spec {
        container {
          name              = "cwagent"
          image             = "${var.cwagent_image_repo}:${var.cwagent_image_tag}"
          image_pull_policy = "Always"
          resources {
            limits = {
              cpu    = "1000m",
              memory = "1000Mi"
            }
            requests = {
              cpu    = "200m",
              memory = "200Mi"
            }
          }
          volume_mount {
            mount_path = "/etc/cwagentconfig"
            name       = "prometheus-cwagentconfig"
          }
          volume_mount {
            mount_path = "/etc/prometheusconfig"
            name       = "prometheus-config"
          }
        }
        volume {
          name = "prometheus-cwagentconfig"
          config_map {
            name = "prometheus-cwagentconfig"
          }
        }
        volume {
          name = "prometheus-config"
          config_map {
            name = "prometheus-config"
          }
        }
        service_account_name             = "cloudwatch-agent"
        termination_grace_period_seconds = 60
      }
    }
  }
}

resource "kubernetes_namespace" "redis" {
  metadata {
    name = "redis-test"
    labels = {
      name = "redis-test"
    }
  }
}

resource "kubernetes_pod" "redis_pod" {
  metadata {
    name      = "redis-instance"
    namespace = "redis-test"
    labels = {
      app = "redis"
    }
  }
  spec {
    container {
      name              = "redis-0"
      image             = "redis:6.0.8-alpine3.12"
      image_pull_policy = "Always"
      port {
        container_port = 6379
      }
    }

    container {
      name              = "redis-exporter-0"
      image             = "oliver006/redis_exporter:v1.11.1-alpine"
      image_pull_policy = "Always"
      port {
        container_port = 9121
        name           = "metrics"
        protocol       = "TCP"
      }
    }
  }
}

resource "kubernetes_service" "redis_service" {
  depends_on = [
    kubernetes_namespace.redis,
    kubernetes_pod.redis_pod,
    aws_eks_node_group.this
  ]
  metadata {
    name      = "my-redis-metrics"
    namespace = "redis-test"
    annotations = {
      "prometheus.io/port"   = "9121"
      "prometheus.io/scrape" = "true"
    }
  }
  spec {
    selector = {
      app = "redis"
    }
    cluster_ip = "None"
    port {
      name        = "metrics"
      port        = 9121
      protocol    = "TCP"
      target_port = "metrics"
    }
  }
}


##########################################
# Template Files
##########################################
locals {
  cwagent_config    = "../../../${var.test_dir}/eks_resources/cwagentconfig.json"
  prometheus_config = "../../../${var.test_dir}/eks_resources/prometheus.yaml"
}

data "template_file" "cwagent_config" {
  template = file(local.cwagent_config)
  vars = {
  }
}

data "template_file" "prometheus_config" {
  template = file(local.prometheus_config)
  vars = {
  }
}

resource "kubernetes_config_map" "cwagentconfig" {
  depends_on = [
    kubernetes_namespace.namespace,
    kubernetes_service_account.cwagentservice
  ]
  metadata {
    name      = "prometheus-cwagentconfig"
    namespace = "amazon-cloudwatch"
  }
  data = {
    "cwagentconfig.json" : data.template_file.cwagent_config.rendered,
  }
}

resource "kubernetes_config_map" "prometheus_config" {
  depends_on = [
    kubernetes_namespace.namespace,
    kubernetes_service_account.cwagentservice
  ]
  metadata {
    name      = "prometheus-config"
    namespace = "amazon-cloudwatch"
  }
  data = {
    "prometheus.yaml" : data.template_file.prometheus_config.rendered
  }
}

resource "kubernetes_service_account" "cwagentservice" {
  depends_on = [kubernetes_namespace.namespace]
  metadata {
    name      = "cloudwatch-agent"
    namespace = "amazon-cloudwatch"
  }
}

resource "kubernetes_cluster_role" "clusterrole" {
  depends_on = [kubernetes_namespace.namespace]
  metadata {
    name = "cloudwatch-agent-role"
  }
  rule {
    verbs      = ["get", "list", "watch"]
    resources  = ["nodes", "nodes/proxy", "services", "endpoints", "pods"]
    api_groups = [""]
  }
  rule {
    verbs             = ["get"]
    non_resource_urls = ["/metrics"]
  }
}

resource "kubernetes_cluster_role_binding" "rolebinding" {
  depends_on = [kubernetes_namespace.namespace]
  metadata {
    name = "cloudwatch-agent-role-binding"
  }
  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "ClusterRole"
    name      = "cloudwatch-agent-role"
  }
  subject {
    kind      = "ServiceAccount"
    name      = "cloudwatch-agent"
    namespace = "amazon-cloudwatch"
  }
}

resource "null_resource" "validator" {
  depends_on = [
    aws_eks_node_group.this,
    kubernetes_deployment.service,
    kubernetes_cluster_role_binding.rolebinding,
    kubernetes_service_account.cwagentservice,
  ]
  provisioner "local-exec" {
    command = <<-EOT
      echo "Validating EKS metrics"
      cd ../../..
      go test ${var.test_dir} -eksClusterName=${aws_eks_cluster.this.name} -computeType=EKS -v -eksDeploymentStrategy=REPLICA
    EOT
  }
}
