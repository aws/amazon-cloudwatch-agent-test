// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "common" {
  source             = "../../../common"
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

  ami_type       = var.ami_type
  capacity_type  = "ON_DEMAND"
  disk_size      = 20
  instance_types = [var.instance_type]
  tags = {
    Owner                                                = "default"
    "kubernetes.io/cluster/${aws_eks_cluster.this.name}" = "owned"
  }

  labels = {
    "vpc.amazonaws.com/efa.present" = "true"
    "nvidia.com/gpu.present"        = "true"
  }

  depends_on = [
    aws_iam_role_policy_attachment.node_AmazonEC2ContainerRegistryReadOnly,
    aws_iam_role_policy_attachment.node_AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy,
    aws_iam_role_policy_attachment.node_CloudWatchAgentServerPolicy
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

resource "helm_release" "efa_plugin" {
  depends_on = [
    kubernetes_namespace.namespace,
    aws_eks_node_group.this,
  ]
  name             = "aws-efa-k8s-device-plugin"
  chart            = "aws-efa-k8s-device-plugin"
  repository       = "https://aws.github.io/eks-charts"
  namespace        = "amazon-cloudwatch"
  create_namespace = true
  wait             = true
  values = [
    <<-EOT
      tolerations:
        - operator: Exists
    EOT
  ]
}
resource "helm_release" "nvidia_device_plugin" {
  depends_on = [
    kubernetes_namespace.namespace,
    aws_eks_node_group.this,
  ]
  name             = "nvidia-device-plugin"
  repository       = "https://nvidia.github.io/k8s-device-plugin"
  chart            = "nvidia-device-plugin"
  version          = "0.17.0"
  namespace        = "amazon-cloudwatch"
  create_namespace = true
  wait             = true
  values = [
    <<-EOT
      tolerations:
        - operator: Exists
    EOT
  ]
}

resource "kubernetes_daemonset" "service" {
  depends_on = [
    kubernetes_namespace.namespace,
    kubernetes_service_account.cwagentservice,
    helm_release.efa_plugin,
    helm_release.nvidia_device_plugin
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
        toleration {
          operator = "Exists"
        }
        init_container {
          name    = "efainit"
          image   = "ubuntu:latest"
          command = ["/bin/bash", "-c"]
          args = [
            # creates EFA data files under an expected location by EFA receiver infiniband/{deviceName}/ports/{port}/hw_counters
            # also changes mod on infiniband folder as the receiver expects the location is only writable by the root (sysfs)
            "cd /work-dir && mkdir -p infiniband/device1/ports/1/hw_counters && echo 10 > infiniband/device1/ports/1/hw_counters/rdma_read_bytes && echo 20 > infiniband/device1/ports/1/hw_counters/rdma_write_bytes && echo 30 > infiniband/device1/ports/1/hw_counters/rdma_write_recv_bytes && echo 100 > infiniband/device1/ports/1/hw_counters/rx_bytes && echo 200 > infiniband/device1/ports/1/hw_counters/rx_drops && echo 300 > infiniband/device1/ports/1/hw_counters/tx_bytes && echo 400 > infiniband/device1/ports/1/hw_counters/unresponsive_remote_events && echo 500 > infiniband/device1/ports/1/hw_counters/retrans_bytes && echo 600 > infiniband/device1/ports/1/hw_counters/retrans_pkts && echo 700 > infiniband/device1/ports/1/hw_counters/impaired_remote_conn_events && echo 800 > infiniband/device1/ports/1/hw_counters/retrans_timeout_events && chmod 755 infiniband"
          ]
          volume_mount {
            name       = "sysefa"
            mount_path = "/work-dir"
          }
        }
        container {
          name              = "cwagent"
          image             = "${var.cwagent_image_repo}:${var.cwagent_image_tag}"
          image_pull_policy = "Always"
          security_context {
            privileged = true
          }
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
          port {
            container_port = 25888
            host_port      = 25888
            protocol       = "UDP"
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
          volume_mount {
            mount_path = "/var/lib/kubelet/pod-resources"
            name       = "kubelet-podresources"
            read_only  = true
          }
          volume_mount {
            mount_path = "/sys/class"
            name       = "sysefa"
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
        volume {
          name = "kubelet-podresources"
          host_path {
            path = "/var/lib/kubelet/pod-resources"
          }
        }
        volume {
          name = "sysefa"
          empty_dir {}
        }
        service_account_name             = "cloudwatch-agent"
        termination_grace_period_seconds = 60
        host_network                     = true
        dns_policy                       = "ClusterFirstWithHostNet"
      }
    }
  }
}

##########################################
# Template Files
##########################################
locals {
  cwagent_config = fileexists("../../../../${var.test_dir}/resources/config.json") ? "../../../../${var.test_dir}/resources/config.json" : "../default_resources/default_amazon_cloudwatch_agent.json"
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
    verbs      = ["get", "list", "watch"]
    resources  = ["pods", "pods/logs", "nodes", "nodes/proxy", "namespaces", "endpoints"]
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
    verbs      = ["list", "watch"]
    resources  = ["services"]
    api_groups = [""]
  }
  rule {
    non_resource_urls = ["/metrics"]
    verbs             = ["get", "list", "watch"]
  }
  rule {
    verbs      = ["list", "watch", "get"]
    resources  = ["endpointslices"]
    api_groups = ["discovery.k8s.io"]
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
  ]

  provisioner "local-exec" {
    command = <<-EOT
      cd ../../../..
      i=0
      while [ $i -lt 10 ]; do
        i=$((i+1))
        go test ${var.test_dir} -eksClusterName=${aws_eks_cluster.this.name} -computeType=EKS -v -eksDeploymentStrategy=DAEMON && exit 0
        sleep 60
      done
      exit 1
    EOT
  }
}