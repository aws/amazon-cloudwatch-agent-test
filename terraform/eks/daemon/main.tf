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
    aws_iam_role_policy_attachment.node_CloudWatchAgentServerPolicy,
  ]
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

# Create a manual storage class for testing
resource "kubernetes_storage_class" "manual" {
  depends_on = [aws_eks_node_group.this]
  metadata {
    name = "manual"
  }
  storage_provisioner = "kubernetes.io/no-provisioner"
  volume_binding_mode = "WaitForFirstConsumer"
}

# Create a sample PersistentVolume for testing PV metrics
resource "kubernetes_persistent_volume" "test_pv" {
  depends_on = [aws_eks_node_group.this, kubernetes_storage_class.manual]
  metadata {
    name = "test-pv-${module.common.testing_id}"
  }
  spec {
    capacity = {
      storage = "1Gi"
    }
    access_modes = ["ReadWriteOnce"]
    persistent_volume_source {
      host_path {
        path = "/tmp/test-pv"
        type = "DirectoryOrCreate"
      }
    }
    storage_class_name = "manual"
    node_affinity {
      required {
        node_selector_term {
          match_expressions {
            key      = "kubernetes.io/os"
            operator = "In"
            values   = ["linux"]
          }
        }
      }
    }
  }
}

# Create a sample PersistentVolumeClaim for testing PVC metrics
resource "kubernetes_persistent_volume_claim" "test_pvc" {
  depends_on = [kubernetes_namespace.namespace, kubernetes_persistent_volume.test_pv]
  metadata {
    name      = "test-pvc-${module.common.testing_id}"
    namespace = "amazon-cloudwatch"
  }
  spec {
    access_modes = ["ReadWriteOnce"]
    resources {
      requests = {
        storage = "1Gi"
      }
    }
    storage_class_name = "manual"
    volume_name        = kubernetes_persistent_volume.test_pv.metadata[0].name
  }
}

# Create a test deployment to back the service and use the PVC
resource "kubernetes_deployment" "test_app" {
  depends_on = [kubernetes_namespace.namespace, kubernetes_persistent_volume_claim.test_pvc]
  metadata {
    name      = "test-app-${module.common.testing_id}"
    namespace = "amazon-cloudwatch"
    labels = {
      app = "test-app"
    }
  }
  spec {
    replicas = 1
    selector {
      match_labels = {
        app = "test-app"
      }
    }
    template {
      metadata {
        labels = {
          app = "test-app"
        }
      }
      spec {
        container {
          name  = "test-app"
          image = "nginx:alpine"
          port {
            container_port = 80
          }
          volume_mount {
            name       = "test-storage"
            mount_path = "/usr/share/nginx/html/data"
          }
        }
        volume {
          name = "test-storage"
          persistent_volume_claim {
            claim_name = kubernetes_persistent_volume_claim.test_pvc.metadata[0].name
          }
        }
      }
    }
  }
}

# Create a sample Service for the Ingress
resource "kubernetes_service" "test_service" {
  depends_on = [kubernetes_namespace.namespace, kubernetes_deployment.test_app]
  metadata {
    name      = "test-service-${module.common.testing_id}"
    namespace = "amazon-cloudwatch"
  }
  spec {
    selector = {
      app = "test-app"
    }
    port {
      port        = 80
      target_port = 80
    }
    type = "ClusterIP"
  }
}

# Create a simple Ingress for testing ingress metrics (no controller needed for metrics)
resource "kubernetes_ingress_v1" "test_ingress" {
  depends_on = [kubernetes_namespace.namespace, kubernetes_service.test_service]
  metadata {
    name      = "test-ingress-${module.common.testing_id}"
    namespace = "amazon-cloudwatch"
  }
  spec {
    rule {
      host = "test.example.com"
      http {
        path {
          path      = "/"
          path_type = "Prefix"
          backend {
            service {
              name = kubernetes_service.test_service.metadata[0].name
              port {
                number = 80
              }
            }
          }
        }
      }
    }
  }
}

# TODO: how do we support different deployment types? Should they be in separate terraform
#       files, and spawn separate tests?
resource "kubernetes_daemonset" "service" {
  depends_on = [
    kubernetes_namespace.namespace,
    kubernetes_config_map.cwagentconfig,
    kubernetes_service_account.cwagentservice,
    aws_eks_node_group.this
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
        node_selector = {
          "kubernetes.io/os" : "linux"
        }
        container {
          name              = "cwagent"
          image             = "${var.cwagent_image_repo}:${var.cwagent_image_tag}"
          image_pull_policy = "Always"
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
            path = "/dev/disk"
          }
        }
        service_account_name             = "cloudwatch-agent"
        termination_grace_period_seconds = 60
      }
    }
  }
}

##########################################
# Template Files
##########################################
locals {
  cwagent_config = fileexists("../../../${var.test_dir}/resources/config.json") ? "../../../${var.test_dir}/resources/config.json" : "./default_resources/default_amazon_cloudwatch_agent.json"
}

data "template_file" "cwagent_config" {
  template = file(local.cwagent_config)
  vars = {
  }
}

resource "kubernetes_config_map" "cwagentconfig" {
  depends_on = [
    kubernetes_namespace.namespace,
    kubernetes_service_account.cwagentservice
  ]
  metadata {
    name      = "cwagentconfig"
    namespace = "amazon-cloudwatch"
  }
  data = {
    "cwagentconfig.json" : data.template_file.cwagent_config.rendered
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
    resources  = ["replicasets", "deployments"]
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
    api_groups = [""]
  }
  rule {
    verbs      = ["create"]
    resources  = ["nodes/stats", "configmaps", "events"]
    api_groups = [""]
  }
  rule {
    verbs          = ["get", "update"]
    resource_names = ["cwagent-clusterleader"]
    resources      = ["configmaps"]
    api_groups     = [""]
  }
  rule {
    verbs      = ["list", "watch", "get"]
    resources  = ["endpointslices"]
    api_groups = ["discovery.k8s.io"]
  }
  rule {
    verbs      = ["list", "watch"]
    resources  = ["persistentvolumes", "persistentvolumeclaims"]
    api_groups = [""]
  }
  rule {
    verbs      = ["list", "watch"]
    resources  = ["ingresses"]
    api_groups = ["networking.k8s.io"]
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
    kubernetes_daemonset.service,
    kubernetes_cluster_role_binding.rolebinding,
    kubernetes_service_account.cwagentservice,
    kubernetes_persistent_volume_claim.test_pvc,
    kubernetes_ingress_v1.test_ingress,
    kubernetes_deployment.test_app,
  ]
  provisioner "local-exec" {
    command = <<-EOT
      echo "Waiting for resources to be ready before running tests..."
      echo "Waiting for CloudWatch agent to start collecting metrics..."
      sleep 300
      echo "Validating EKS metrics/logs"
      cd ../../..
      go test ${var.test_dir} -eksClusterName=${aws_eks_cluster.this.name} -computeType=EKS -v -eksDeploymentStrategy=DAEMON
    EOT
  }
}
