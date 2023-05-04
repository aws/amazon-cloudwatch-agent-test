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
  name = aws_eks_cluster.cluster.name
}

resource "kubernetes_namespace" "namespace" {
  depends_on = [aws_eks_cluster.cluster]
  metadata {
    name = "amazon-cloudwatch"
  }
}

resource "aws_eks_cluster" "cluster" {
  version                   = "1.23" # TODO: parameterize this
  name                      = "cwagent-integ-test-eks-${module.common.testing_id}"
  role_arn                  = module.basic_components.role_arn
  vpc_config {
    subnet_ids         = module.basic_components.public_subnet_ids
    security_group_ids = [module.basic_components.security_group]
  }
}

resource "kubernetes_daemonset" "service" {
  depends_on = [kubernetes_namespace.namespace]
  metadata {
    name      = "cloudwatch-agent"
    namespace = "amazon-cloudwatch"
  }
  spec {
    selector {
      match_labels = {
        "name": "cloudwatch-agent"
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

resource "kubernetes_config_map" "cwagentconfig" {
  depends_on = [kubernetes_namespace.namespace]
  metadata {
    name = "cwagentconfig"
    namespace = "amazon-cloudwatch"
  }
  data = {
    "configuration": file("${path.module}/resources/configmap.yml")
  }
}

resource "kubernetes_service_account" "cwagentservice" {
  depends_on = [kubernetes_namespace.namespace]
  metadata {
    name = "cloudwatch-agent"
    namespace = "amazon-cloudwatch"
  }
}

resource "kubernetes_cluster_role" "clusterrole" {
  depends_on = [kubernetes_namespace.namespace]
  metadata {
    name = "cloudwatch-agent-role"
  }
  rule {
    verbs = ["list", "watch"]
    resources = ["pods", "nodes", "endpoints"]
    api_groups = [""]
  }
  rule {
    verbs = ["list", "watch"]
    resources = ["replicasets"]
    api_groups = ["apps"]
  }
  rule {
    verbs = ["list", "watch"]
    resources = ["jobs"]
    api_groups = ["batch"]
  }
  rule {
    verbs = ["get"]
    resources = ["nodes/proxy"]
    api_groups = [""]
  }
  rule {
    verbs = ["create"]
    resources = ["nodes/stats", "configmaps", "events"]
    api_groups = [""]
  }
  rule {
    verbs = ["get", "update"]
    resource_names = ["cwagent-clusterleader"]
    resources = ["configmaps"]
    api_groups = [""]
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
    kind = "ServiceAccount"
    name = "cloudwatch-agent"
    namespace = "amazon-cloudwatch"
  }
}

resource "null_resource" "validator" {
  depends_on = [
    kubernetes_namespace.namespace,
    kubernetes_daemonset.service,
    kubernetes_config_map.cwagentconfig,
    kubernetes_service_account.cwagentservice,
    kubernetes_cluster_role.clusterrole,
    kubernetes_cluster_role_binding.rolebinding
  ]
  provisioner "local-exec" {
    command = <<-EOT
      echo "Validating EKS metrics/logs"
      cd ../../..
      go test ${var.test_dir} -clusterArn=${aws_eks_cluster.cluster.arn} -computeType=EKS -v
    EOT
  }
}
