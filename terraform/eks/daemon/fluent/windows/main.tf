// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "common" {
  source             = "../../../../common"
  cwagent_image_repo = var.cwagent_image_repo
  cwagent_image_tag  = var.cwagent_image_tag
}

module "basic_components" {
  source = "../../../../basic_components"

  region = var.region
}

resource "aws_eks_cluster" "cluster" {
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

locals {
  aws_eks      = "aws eks --region ${var.region}"
  cluster_name = aws_eks_cluster.cluster.name
}

data "aws_eks_cluster_auth" "cluster_auth" {
  name = aws_eks_cluster.cluster.name
}

data "aws_caller_identity" "account_id" {}

## EKS Cluster Addon
resource "aws_eks_addon" "eks_windows_addon" {
  cluster_name = aws_eks_cluster.cluster.name
  addon_name   = "vpc-cni"
}

## Enable VPC CNI Windows Support

resource "kubernetes_config_map_v1_data" "amazon_vpc_cni_windows" {
  depends_on = [
    aws_eks_cluster.cluster,
    aws_eks_addon.eks_windows_addon
  ]
  metadata {
    name      = "amazon-vpc-cni"
    namespace = "kube-system"
  }

  force = true

  data = {
    enable-windows-ipam : "true"
  }
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

## AWS CONFIGMAP

resource "kubernetes_config_map" "configmap" {
  data = {
    "mapRoles" = <<EOT
- groups:
  - system:bootstrappers
  - system:nodes
  rolearn: arn:aws:iam::${data.aws_caller_identity.account_id.account_id}:role/${aws_iam_role.node_role.name}
  username: system:node:{{EC2PrivateDNSName}}
- groups:
  - eks:kube-proxy-windows
  - system:bootstrappers
  - system:nodes
  rolearn: arn:aws:iam::${data.aws_caller_identity.account_id.account_id}:role/${aws_iam_role.node_role.name}
  username: system:node:{{EC2PrivateDNSName}}
- groups:
  - system:masters
  rolearn: arn:aws:iam::${data.aws_caller_identity.account_id.account_id}:role/Admin-Windows

EOT
  }

  metadata {
    name      = "aws-auth"
    namespace = "kube-system"
  }
}

# EKS Node Groups
resource "aws_eks_node_group" "node_group" {
  cluster_name    = aws_eks_cluster.cluster.name
  node_group_name = "cwagent-eks-integ-node"
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
    aws_iam_role_policy_attachment.node_CloudWatchAgentServerPolicy,
  ]
}

# EKS Windows Node Groups
resource "aws_eks_node_group" "node_group_windows" {
  cluster_name    = aws_eks_cluster.cluster.name
  node_group_name = "${local.cluster_name}-windows-node"
  node_role_arn   = aws_iam_role.node_role.arn
  subnet_ids      = module.basic_components.public_subnet_ids

  scaling_config {
    desired_size = 1
    max_size     = 1
    min_size     = 1
  }

  ami_type       = var.windows_ami_type
  capacity_type  = "ON_DEMAND"
  disk_size      = 50
  instance_types = ["t3.large"]

  depends_on = [
    aws_iam_role_policy_attachment.node_CloudWatchAgentServerPolicy,
    aws_iam_role_policy_attachment.node_AmazonEC2ContainerRegistryReadOnly,
    aws_iam_role_policy_attachment.node_AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy
  ]
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
    verbs      = ["list", "watch"]
    resources  = ["pods", "nodes", "endpoints"]
    api_groups = [""]
  }
  rule {
    verbs      = ["list", "watch"]
    resources  = ["replicasets"]
    api_groups = ["apps"]
  }
  rule {
    verbs      = ["list", "watch"]
    resources  = ["jobs"]
    api_groups = ["batch"]
  }
  rule {
    verbs      = ["get"]
    resources  = ["nodes/proxy"]
    api_groups = ["get"]
  }
  rule {
    verbs      = ["create", "get"]
    resources  = ["nodes/stats", "configmaps", "events"]
    api_groups = [""]
  }
  rule {
    verbs          = ["get", "update"]
    resource_names = ["cwagent-clusterleader"]
    resources      = ["configmaps"]
    api_groups     = [""]
  }
}

resource "kubernetes_cluster_role_binding" "rolebinding" {
  depends_on = [
    kubernetes_service_account.cwagentservice,
    kubernetes_cluster_role.clusterrole
  ]
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

resource "kubernetes_config_map" "cwagentconfig" {
  metadata {
    name      = "cwagentconfig"
    namespace = "amazon-cloudwatch"
  }
  data = {
    "cwagentconfig.json" = <<EOF
    {
      "agent": {
        "region": "${var.region}"
      },
      "logs": {
        "metrics_collected": {
          "kubernetes": {
            "cluster_name": "${aws_eks_cluster.cluster.name}",
            "metrics_collection_interval": 60
          }
        },
        "force_flush_interval": 5
      }
    }
    EOF
  }
}

resource "kubernetes_daemonset" "agent_daemon" {
  depends_on = [
    kubernetes_config_map.cwagentconfig
  ]
  metadata {
    name      = "cloudwatch-agent"
    namespace = "amazon-cloudwatch"
  }
  spec {
    selector {
      match_labels = {
        "name" : "cloudwatch-agent"
      }
    }
    template {
      metadata {
        labels = {
          "name" : "cloudwatch-agent"
        }
      }
      spec {
        container {
          name  = "cloudwatch-agent"
          image = "public.ecr.aws/cloudwatch-agent/cloudwatch-agent:1.247359.1b252618"
          resources {
            limits = {
              "cpu" : "200m",
              "memory" : "200Mi"
            }
            requests = {
              "cpu" : "200m",
              "memory" : "200Mi"
            }
          }
          env {
            name = "HOST_IP"
            value_from {
              field_ref {
                field_path = "status.hostIP"
              }
            }
          }
          env {
            name = "HOST_NAME"
            value_from {
              field_ref {
                field_path = "spec.nodeName"
              }
            }
          }
          env {
            name = "K8S_NAMESPACE"
            value_from {
              field_ref {
                field_path = "metadata.namespace"
              }
            }
          }
          env {
            name  = "CI_VERSION"
            value = "k8s/1.3.15"
          }
          volume_mount {
            mount_path = "/etc/cwagentconfig"
            name       = "cwagentconfig"
          }
          volume_mount {
            mount_path = "/rootfs"
            name       = "rootfs"
            read_only  = true
          }
          volume_mount {
            mount_path = "/var/run/docker.sock"
            name       = "dockersock"
            read_only  = true
          }
          volume_mount {
            mount_path = "/var/lib/docker"
            name       = "varlibdocker"
            read_only  = true
          }
          volume_mount {
            mount_path = "/run/containerd/containerd.sock"
            name       = "containerdsock"
            read_only  = true
          }
          volume_mount {
            mount_path = "/sys"
            name       = "sys"
            read_only  = true
          }
          volume_mount {
            mount_path = "/dev/disk"
            name       = "devdisk"
            read_only  = true
          }
        }
        node_selector = {
          "kubernetes.io/os" : "linux"
        }
        volume {
          name = "cwagentconfig"
          config_map {
            name = "cwagentconfig"
          }
        }
        volume {
          name = "rootfs"
          host_path {
            path = "/"
          }
        }
        volume {
          name = "dockersock"
          host_path {
            path = "/var/run/docker.sock"
          }
        }
        volume {
          name = "varlibdocker"
          host_path {
            path = "/var/lib/docker"
          }
        }
        volume {
          name = "containerdsock"
          host_path {
            path = "/run/containerd/containerd.sock"
          }
        }
        volume {
          name = "sys"
          host_path {
            path = "/sys"
          }
        }
        volume {
          name = "devdisk"
          host_path {
            path = "/dev/disk/"
          }
        }
        termination_grace_period_seconds = 60
        service_account_name             = "cloudwatch-agent"
      }
    }
  }
}

resource "null_resource" "kubectl" {
  depends_on = [
    aws_eks_node_group.node_group_windows
  ]
  provisioner "local-exec" {
    command = <<-EOT
      ${local.aws_eks} update-kubeconfig --name ${aws_eks_cluster.cluster.name}
      ${local.aws_eks} list-clusters --output text
      ${local.aws_eks} describe-cluster --name ${aws_eks_cluster.cluster.name} --output text
    EOT
  }
}

resource "kubernetes_config_map" "cluster_info" {
  depends_on = [
    null_resource.kubectl
  ]
  metadata {
    name      = "fluent-bit-cluster-info"
    namespace = "amazon-cloudwatch"
  }
  data = {
    "cluster.name" = aws_eks_cluster.cluster.name
    "logs.region"  = var.region
    "http.server"  = "Off"
    "http.port"    = "2020"
    "read.head"    = "Off"
    "read.tail"    = "On"
  }
}

resource "kubernetes_service_account" "fluentbit_service" {
  metadata {
    name      = "fluent-bit"
    namespace = "amazon-cloudwatch"
  }
}

resource "kubernetes_cluster_role" "fluentbit_clusterrole" {
  metadata {
    name = "fluent-bit-role"
  }
  rule {
    non_resource_urls = ["/metrics"]
    verbs             = ["get"]
  }
  rule {
    verbs      = ["get", "list", "watch"]
    resources  = ["namespaces", "pods", "pods/logs", "nodes", "nodes/proxy"]
    api_groups = [""]
  }
}

resource "kubernetes_cluster_role_binding" "fluentbit_rolebinding" {
  depends_on = [
    kubernetes_service_account.fluentbit_service,
    kubernetes_cluster_role.fluentbit_clusterrole,
  ]
  metadata {
    name = "fluent-bit-role-binding"
  }
  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "ClusterRole"
    name      = "fluent-bit-role"
  }
  subject {
    kind      = "ServiceAccount"
    name      = "fluent-bit"
    namespace = "amazon-cloudwatch"
  }
}

resource "null_resource" "fluentbit-windows" {
  depends_on = [
    aws_eks_node_group.node_group_windows,
    null_resource.kubectl
  ]

  provisioner "local-exec" {
    command = <<-EOT
      curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
      chmod +x kubectl
      ./kubectl apply -f ./../../../default_resources/fluenbit-windows-configmap.yaml
      ./kubectl apply -f ./../../../default_resources/fluenbit-windows-daemonset.yaml
      ./kubectl rollout status daemonset fluent-bit-windows -n amazon-cloudwatch --timeout 600s
      sed -e 's+WINDOWS_SERVER_VERSION+${var.windows_os_version}+' -e 's+REPLICAS+1+' ./../../../default_resources/test-sample-windows.yaml | ./kubectl apply -f -
      ./kubectl rollout status deployment windows-test-deployment --timeout 600s
      sleep 120
    EOT
  }
}

resource "null_resource" "validator" {
  depends_on = [
    null_resource.fluentbit-windows,
    kubernetes_cluster_role_binding.fluentbit_rolebinding
  ]
  provisioner "local-exec" {
    command = <<-EOT
      echo "Validating EKS fluentbit logs"
      cd ../../../../../..
      go test ${var.test_dir} -eksClusterName=${aws_eks_cluster.cluster.name} -computeType=EKS -v -eksDeploymentStrategy=DAEMON -instancePlatform=windows
    EOT
  }
}

#resource "null_resource" "clean-up" {
#  depends_on = [
#
#    null_resource.fluentbit-windows,
#    kubernetes_cluster_role_binding.fluentbit_rolebinding,
#    null_resource.validator
#  ]
#  provisioner "local-exec" {
#    command = <<-EOT
#      echo "Cleaning up EKS fluentbit applications"
#      sed -e 's+WINDOWS_SERVER_VERSION+${var.windows_os_version}+' -e 's+REPLICAS+1+' ./../../../default_resources/test-sample-windows.yaml | ./kubectl apply -f -
#      ./kubectl wait --for=delete deployment.apps windows-test-deployment --timeout 600s
#      ./kubectl delete -f ./../../../default_resources/fluenbit-windows-daemonset.yaml
#      ./kubectl wait --for=delete daemonset.apps fluent-bit-windows --timeout 600s
#    EOT
#  }
#}
