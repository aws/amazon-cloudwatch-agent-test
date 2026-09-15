// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "common" {
  source = "../../common"
}

data "azurerm_subnet" "selected" {
  name                 = var.azure_subnet_name
  virtual_network_name = var.azure_vnet_name
  resource_group_name  = var.azure_resource_group
}

#####################################################################
# AKS cluster with OIDC issuer (for AWS cross-cloud web-identity)
#####################################################################
resource "azurerm_kubernetes_cluster" "cwagent" {
  name                = "cwa-aks-integ-${module.common.testing_id}"
  location            = var.azure_location
  resource_group_name = var.azure_resource_group
  dns_prefix          = "cwa-aks-${module.common.testing_id}"
  kubernetes_version  = var.kubernetes_version

  oidc_issuer_enabled       = true
  workload_identity_enabled = true

  # Terraform drives the cluster over the public API server, so restrict it to the runner that created it.
  # runner_ip is required, so there is no path where this silently ends up open to all.
  api_server_access_profile {
    authorized_ip_ranges = [var.runner_ip]
  }

  identity {
    type = "SystemAssigned"
  }

  default_node_pool {
    name                        = "default"
    node_count                  = var.aks_node_count
    vm_size                     = var.aks_node_vm_size
    os_disk_size_gb             = 50
    temporary_name_for_rotation = "tmpdefault"
    vnet_subnet_id              = data.azurerm_subnet.selected.id
  }

  network_profile {
    network_plugin = "azure"
  }
}

#####################################################################
# AWS IAM: trust AKS OIDC issuer for cross-cloud federation
#####################################################################
data "tls_certificate" "aks_oidc" {
  url = azurerm_kubernetes_cluster.cwagent.oidc_issuer_url
}

resource "aws_iam_openid_connect_provider" "aks" {
  url             = azurerm_kubernetes_cluster.cwagent.oidc_issuer_url
  client_id_list  = ["sts.amazonaws.com"]
  thumbprint_list = [data.tls_certificate.aks_oidc.certificates[0].sha1_fingerprint]
}

locals {
  aks_oidc_issuer_host = replace(azurerm_kubernetes_cluster.cwagent.oidc_issuer_url, "https://", "")
  namespace            = "amazon-cloudwatch"
  service_account_name = "cloudwatch-agent"
  cwagent_role_name    = "cwa-aks-integ-role-${module.common.testing_id}"

  # test_mode == "containerinsights" swaps the OTLP load-gen topology for the CI node+cluster topology.
  is_ci = var.test_mode == "containerinsights"

  # Must match serviceName in test/azure/aks/aks_test.go -- the test derives the expected log stream
  # and the trace query filter from it.
  load_gen_service_name     = "aks-otlp-test-service"
  load_gen_duration_seconds = 180
}

data "aws_iam_policy_document" "cwagent_assume_role" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [aws_iam_openid_connect_provider.aks.arn]
    }

    condition {
      test     = "StringEquals"
      variable = "${local.aks_oidc_issuer_host}:sub"
      values   = ["system:serviceaccount:${local.namespace}:${local.service_account_name}"]
    }

    condition {
      test     = "StringEquals"
      variable = "${local.aks_oidc_issuer_host}:aud"
      values   = ["sts.amazonaws.com"]
    }
  }

}

resource "aws_iam_role" "cwagent" {
  name               = local.cwagent_role_name
  assume_role_policy = data.aws_iam_policy_document.cwagent_assume_role.json
}

# The agent's own writes come from the same AWS-managed policy customers are told to use, so a green run
# also proves that documented policy is sufficient over the AKS workload-identity path.
resource "aws_iam_role_policy_attachment" "cwagent_server_policy" {
  role       = aws_iam_role.cwagent.name
  policy_arn = "arn:aws:iam::aws:policy/CloudWatchAgentServerPolicy"
}

# No inline policy: the AKS test binary runs on the runner under its own credentials, so this role needs
# agent writes only -- and CloudWatchAgentServerPolicy alone covers them, OTLP traces included.

#####################################################################
# Kubernetes resources: deploy CWA DaemonSet from ECR image
#####################################################################
resource "kubernetes_namespace" "cwagent" {
  metadata {
    name = local.namespace
  }
}

resource "kubernetes_service_account" "cwagent" {
  metadata {
    name      = local.service_account_name
    namespace = kubernetes_namespace.cwagent.metadata[0].name
  }
}

resource "kubernetes_cluster_role" "cwagent" {
  metadata {
    name = "cwa-aks-integ-${module.common.testing_id}"
  }

  rule {
    api_groups = [""]
    resources  = ["pods", "nodes", "endpoints", "services", "namespaces"]
    verbs      = ["list", "watch", "get"]
  }
  rule {
    api_groups = ["apps"]
    resources  = ["replicasets", "daemonsets", "deployments"]
    verbs      = ["list", "watch", "get"]
  }
  rule {
    api_groups = ["batch"]
    resources  = ["jobs"]
    verbs      = ["list", "watch", "get"]
  }
}

resource "kubernetes_cluster_role_binding" "cwagent" {
  metadata {
    name = "cwa-aks-integ-${module.common.testing_id}"
  }
  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "ClusterRole"
    name      = kubernetes_cluster_role.cwagent.metadata[0].name
  }
  subject {
    kind      = "ServiceAccount"
    name      = kubernetes_service_account.cwagent.metadata[0].name
    namespace = kubernetes_namespace.cwagent.metadata[0].name
  }
}

# ECR pull secret so AKS nodes can pull the CWA image from AWS ECR.
# The 12h auth token is fetched here with the runner's AWS credentials rather
# than passed in as a variable, which cannot survive the workflow's shell quoting.
# The integration-test image is published to us-west-2 only, while the job's
# CloudWatch region may differ -- pin the registry host to the ECR region.
locals {
  cwagent_image_repo = replace(var.cwagent_image_repo, "/\\.ecr\\.[a-z0-9-]+\\./", ".ecr.${var.ecr_region}.")
}

data "aws_ecr_authorization_token" "ecr" {
  provider = aws.ecr
}

resource "kubernetes_secret" "ecr_pull" {
  metadata {
    name      = "ecr-pull-secret"
    namespace = kubernetes_namespace.cwagent.metadata[0].name
  }
  type = "kubernetes.io/dockerconfigjson"
  data = {
    ".dockerconfigjson" = jsonencode({
      auths = {
        (split("/", local.cwagent_image_repo)[0]) = {
          auth = data.aws_ecr_authorization_token.ecr.authorization_token
        }
      }
    })
  }
}

resource "kubernetes_daemon_set_v1" "cwagent" {
  metadata {
    name      = "cloudwatch-agent"
    namespace = kubernetes_namespace.cwagent.metadata[0].name
  }

  spec {
    selector {
      match_labels = { app = "cloudwatch-agent" }
    }

    template {
      metadata {
        labels = { app = "cloudwatch-agent" }
      }

      spec {
        service_account_name = kubernetes_service_account.cwagent.metadata[0].name
        host_network         = true
        dns_policy           = "ClusterFirstWithHostNet"

        image_pull_secrets {
          name = kubernetes_secret.ecr_pull.metadata[0].name
        }

        container {
          name              = "cloudwatch-agent"
          image             = "${local.cwagent_image_repo}:${var.cwagent_image_tag}"
          image_pull_policy = "Always"

          env {
            name  = "AWS_REGION"
            value = var.region
          }
          env {
            name  = "AWS_WEB_IDENTITY_TOKEN_FILE"
            value = "/var/run/secrets/aws/token"
          }
          env {
            name  = "AWS_ROLE_ARN"
            value = aws_iam_role.cwagent.arn
          }
          # CWAGENT_ROLE_ARN is deliberately unset. It only feeds sigv4auth's role_arn, and leaving that
          # empty makes the extension fall through to the default credential chain, which picks up the
          # projected token via AWS_ROLE_ARN + AWS_WEB_IDENTITY_TOKEN_FILE. Setting it would layer a
          # redundant sts:AssumeRole of the same role on top of the session we already have.
          env {
            name  = "RUN_IN_CONTAINER"
            value = "True"
          }
          # Explicit AKS signal so mode detection selects the Azure credential/region
          # path without depending on an IMDS probe from the pod.
          env {
            name  = "RUN_IN_AKS"
            value = "True"
          }
          # OTLP mode uses the built-in default:otel config. CI mode drops it so the agent
          # translates the JSON mounted at /etc/cwagentconfig below.
          dynamic "env" {
            for_each = local.is_ci ? [] : [1]
            content {
              name  = "USE_DEFAULT_CONFIG"
              value = "otel"
            }
          }
          env {
            name = "K8S_NODE_NAME"
            value_from {
              field_ref {
                field_path = "spec.nodeName"
              }
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

          volume_mount {
            name       = "aws-token"
            mount_path = "/var/run/secrets/aws"
            read_only  = true
          }
          volume_mount {
            name       = "rootfs"
            mount_path = "/rootfs"
            read_only  = true
          }
          dynamic "volume_mount" {
            for_each = local.is_ci ? [1] : []
            content {
              name       = "cwagentconfig"
              mount_path = "/etc/cwagentconfig"
              read_only  = true
            }
          }
          dynamic "volume_mount" {
            for_each = local.is_ci ? [1] : []
            content {
              name       = "agent-client-cert"
              mount_path = "/etc/amazon-cloudwatch-observability-agent-client-cert"
              read_only  = true
            }
          }
        }

        volume {
          name = "aws-token"
          projected {
            sources {
              service_account_token {
                audience           = "sts.amazonaws.com"
                expiration_seconds = 86400
                path               = "token"
              }
            }
          }
        }
        volume {
          name = "rootfs"
          host_path {
            path = "/"
          }
        }
        dynamic "volume" {
          for_each = local.is_ci ? [1] : []
          content {
            name = "cwagentconfig"
            config_map {
              name = kubernetes_config_map.ci_node[0].metadata[0].name
            }
          }
        }
        dynamic "volume" {
          for_each = local.is_ci ? [1] : []
          content {
            name = "agent-client-cert"
            secret {
              secret_name = kubernetes_secret.ci_scrape_ca[0].metadata[0].name
            }
          }
        }
      }
    }
  }

  depends_on = [
    kubernetes_cluster_role_binding.cwagent,
    aws_iam_role_policy_attachment.cwagent_server_policy,
  ]
}

#####################################################################
# Load generator: pushes OTLP to localhost:4318 for 3 min via hostNetwork
#####################################################################
# See otlp_load_generator.sh for what the payloads carry and why.
resource "kubernetes_job_v1" "otlp_load" {
  count = local.is_ci ? 0 : 1
  metadata {
    name      = "otlp-load-generator"
    namespace = kubernetes_namespace.cwagent.metadata[0].name
  }

  spec {
    backoff_limit = 0

    template {
      metadata {
        labels = { app = "otlp-load" }
      }

      spec {
        host_network   = true
        dns_policy     = "ClusterFirstWithHostNet"
        restart_policy = "Never"

        container {
          name    = "load-gen"
          image   = "curlimages/curl:8.8.0"
          command = ["/bin/sh", "-c"]
          args = [templatefile("${path.module}/otlp_load_generator.sh", {
            service_name     = local.load_gen_service_name
            instance_id      = azurerm_kubernetes_cluster.cwagent.name
            endpoint         = "http://127.0.0.1:4318"
            duration_seconds = local.load_gen_duration_seconds
          })]
        }
      }
    }
  }

  wait_for_completion = true
  timeouts {
    create = "10m"
  }

  depends_on = [kubernetes_daemon_set_v1.cwagent]
}

#####################################################################
# Diagnostics: surface agent pod state and logs in the job output so
# delivery failures are debuggable after the cluster is destroyed.
#####################################################################
resource "local_sensitive_file" "kubeconfig" {
  content         = azurerm_kubernetes_cluster.cwagent.kube_config_raw
  filename        = "${path.module}/kubeconfig"
  file_permission = "0600"
}

resource "null_resource" "agent_diagnostics" {
  provisioner "local-exec" {
    command = <<-EOT
      kubectl --kubeconfig='${local_sensitive_file.kubeconfig.filename}' get pods -n amazon-cloudwatch -o wide || true
      kubectl --kubeconfig='${local_sensitive_file.kubeconfig.filename}' logs -n amazon-cloudwatch -l app=cloudwatch-agent --tail=200 --prefix || true
    EOT
  }

  depends_on = [kubernetes_daemon_set_v1.cwagent, kubernetes_job_v1.otlp_load]
}

#####################################################################
# Run Go integration test from the runner (validates CloudWatch)
#####################################################################
resource "null_resource" "integration_test" {
  provisioner "local-exec" {
    working_dir = "${path.module}/../../../"
    command     = <<-EOT
      go test -tags integration ${var.test_dir} -p 1 -timeout 30m \
        -computeType=AKS \
        -region=${var.region} \
        -cwaCommitSha=${var.cwa_github_sha} \
        -aksClusterName=${azurerm_kubernetes_cluster.cwagent.name} \
        -v
    EOT

    environment = {
      AWS_REGION = var.region
    }
  }

  depends_on = [kubernetes_daemon_set_v1.cwagent, kubernetes_job_v1.otlp_load, kubernetes_deployment_v1.cluster_scraper, null_resource.ci_extra_manifests, null_resource.agent_diagnostics]
}

#####################################################################
# Container Insights topology (test_mode == "containerinsights").
# All resources below are count-gated so OTLP-mode applies are unchanged.
#####################################################################

# Agent JSON configs, cluster_name placeholder replaced with the real cluster name.
resource "kubernetes_config_map" "ci_node" {
  count = local.is_ci ? 1 : 0
  metadata {
    name      = "cwagentconfig"
    namespace = kubernetes_namespace.cwagent.metadata[0].name
  }
  data = {
    "cwagentconfig.json" = replace(
      file("${path.module}/../../../${var.test_dir}/resources/ci_node.json"),
      "AKS_CLUSTER_NAME", azurerm_kubernetes_cluster.cwagent.name,
    )
  }
}

resource "kubernetes_config_map" "ci_cluster" {
  count = local.is_ci ? 1 : 0
  metadata {
    name      = "cwagentconfig-cluster-scraper"
    namespace = kubernetes_namespace.cwagent.metadata[0].name
  }
  data = {
    "cwagentconfig.json" = replace(
      file("${path.module}/../../../${var.test_dir}/resources/ci_cluster.json"),
      "AKS_CLUSTER_NAME", azurerm_kubernetes_cluster.cwagent.name,
    )
  }
}

#####################################################################
# Self-signed CA + server cert for the KSM / node-exporter TLS scrapes.
# The agent trusts the CA (mounted at the two agent cert paths); KSM and
# node-exporter serve the leaf via exporter-toolkit web-config.
#####################################################################
resource "tls_private_key" "ci_ca" {
  count     = local.is_ci ? 1 : 0
  algorithm = "RSA"
  rsa_bits  = 2048
}

resource "tls_self_signed_cert" "ci_ca" {
  count             = local.is_ci ? 1 : 0
  private_key_pem   = tls_private_key.ci_ca[0].private_key_pem
  is_ca_certificate = true
  subject {
    common_name = "cwa-aks-ci-ca"
  }
  validity_period_hours = 24
  allowed_uses          = ["cert_signing", "crl_signing"]
}

resource "tls_private_key" "ci_server" {
  count     = local.is_ci ? 1 : 0
  algorithm = "RSA"
  rsa_bits  = 2048
}

resource "tls_cert_request" "ci_server" {
  count           = local.is_ci ? 1 : 0
  private_key_pem = tls_private_key.ci_server[0].private_key_pem
  subject {
    common_name = "kube-state-metrics.${local.namespace}.svc"
  }
  dns_names = [
    "kube-state-metrics",
    "kube-state-metrics.${local.namespace}.svc",
    "node-exporter-service",
    "node-exporter-service.${local.namespace}.svc",
  ]
}

resource "tls_locally_signed_cert" "ci_server" {
  count                 = local.is_ci ? 1 : 0
  cert_request_pem      = tls_cert_request.ci_server[0].cert_request_pem
  ca_private_key_pem    = tls_private_key.ci_ca[0].private_key_pem
  ca_cert_pem           = tls_self_signed_cert.ci_ca[0].cert_pem
  validity_period_hours = 24
  allowed_uses          = ["server_auth"]
}

# CA the agent trusts. Mounted at BOTH agent cert paths (node uses the
# -client-cert path for node-exporter; cluster-scraper uses -cert for KSM).
resource "kubernetes_secret" "ci_scrape_ca" {
  count = local.is_ci ? 1 : 0
  metadata {
    name      = "ci-scrape-ca"
    namespace = kubernetes_namespace.cwagent.metadata[0].name
  }
  data = {
    "tls-ca.crt" = tls_self_signed_cert.ci_ca[0].cert_pem
  }
}

# Server leaf + exporter-toolkit web-config shared by KSM and node-exporter.
resource "kubernetes_secret" "ci_server_cert" {
  count = local.is_ci ? 1 : 0
  metadata {
    name      = "ci-server-cert"
    namespace = kubernetes_namespace.cwagent.metadata[0].name
  }
  data = {
    "server.crt"      = tls_locally_signed_cert.ci_server[0].cert_pem
    "server.key"      = tls_private_key.ci_server[0].private_key_pem
    "web-config.yaml" = <<-EOT
      tls_server_config:
        cert_file: /tls/server.crt
        key_file: /tls/server.key
    EOT
  }
}

#####################################################################
# kube-state-metrics (cluster metrics: kube_node_info / kube_pod_info)
#####################################################################
resource "kubernetes_cluster_role" "ksm" {
  count = local.is_ci ? 1 : 0
  metadata {
    name = "cwa-aks-ci-ksm-${module.common.testing_id}"
  }
  rule {
    api_groups = [""]
    resources  = ["pods", "nodes", "namespaces", "services", "endpoints"]
    verbs      = ["list", "watch"]
  }
  rule {
    api_groups = ["apps"]
    resources  = ["deployments", "replicasets", "daemonsets", "statefulsets"]
    verbs      = ["list", "watch"]
  }
  rule {
    api_groups = ["batch"]
    resources  = ["jobs", "cronjobs"]
    verbs      = ["list", "watch"]
  }
}

resource "kubernetes_cluster_role_binding" "ksm" {
  count = local.is_ci ? 1 : 0
  metadata {
    name = "cwa-aks-ci-ksm-${module.common.testing_id}"
  }
  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "ClusterRole"
    name      = kubernetes_cluster_role.ksm[0].metadata[0].name
  }
  subject {
    kind      = "ServiceAccount"
    name      = kubernetes_service_account.cwagent.metadata[0].name
    namespace = local.namespace
  }
}

resource "kubernetes_deployment_v1" "ksm" {
  count = local.is_ci ? 1 : 0
  metadata {
    name      = "kube-state-metrics"
    namespace = local.namespace
    labels    = { app = "kube-state-metrics" }
  }
  spec {
    replicas = 1
    selector {
      match_labels = { app = "kube-state-metrics" }
    }
    template {
      metadata {
        labels = { app = "kube-state-metrics" }
      }
      spec {
        service_account_name = kubernetes_service_account.cwagent.metadata[0].name
        image_pull_secrets {
          name = kubernetes_secret.ecr_pull.metadata[0].name
        }
        container {
          name  = "kube-state-metrics"
          image = "registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.13.0"
          args = [
            "--port=8443",
            "--web.config.file=/web/web-config.yaml",
          ]
          port {
            name           = "https"
            container_port = 8443
          }
          volume_mount {
            name       = "tls"
            mount_path = "/tls"
            read_only  = true
          }
          volume_mount {
            name       = "web"
            mount_path = "/web"
            read_only  = true
          }
        }
        volume {
          name = "tls"
          secret {
            secret_name = kubernetes_secret.ci_server_cert[0].metadata[0].name
          }
        }
        volume {
          name = "web"
          secret {
            secret_name = kubernetes_secret.ci_server_cert[0].metadata[0].name
            items {
              key  = "web-config.yaml"
              path = "web-config.yaml"
            }
          }
        }
      }
    }
  }
}

resource "kubernetes_service" "ksm" {
  count = local.is_ci ? 1 : 0
  metadata {
    name      = "kube-state-metrics"
    namespace = local.namespace
  }
  spec {
    selector = { app = "kube-state-metrics" }
    port {
      name        = "https"
      port        = 8443
      target_port = 8443
    }
  }
}

#####################################################################
# node-exporter (node metrics: node_cpu_seconds_total, node_memory_*)
#####################################################################
resource "kubernetes_daemon_set_v1" "node_exporter" {
  count = local.is_ci ? 1 : 0
  metadata {
    name      = "node-exporter"
    namespace = local.namespace
    labels    = { app = "node-exporter" }
  }
  spec {
    selector {
      match_labels = { app = "node-exporter" }
    }
    template {
      metadata {
        labels = { app = "node-exporter" }
      }
      spec {
        host_network = true
        host_pid     = true
        image_pull_secrets {
          name = kubernetes_secret.ecr_pull.metadata[0].name
        }
        container {
          name  = "node-exporter"
          image = "quay.io/prometheus/node-exporter:v1.8.2"
          args = [
            "--web.listen-address=:9487",
            "--web.config.file=/web/web-config.yaml",
            "--path.rootfs=/host/root",
          ]
          port {
            name           = "https"
            container_port = 9487
          }
          volume_mount {
            name       = "tls"
            mount_path = "/tls"
            read_only  = true
          }
          volume_mount {
            name       = "web"
            mount_path = "/web"
            read_only  = true
          }
          volume_mount {
            name              = "root"
            mount_path        = "/host/root"
            read_only         = true
            mount_propagation = "HostToContainer"
          }
        }
        volume {
          name = "tls"
          secret {
            secret_name = kubernetes_secret.ci_server_cert[0].metadata[0].name
          }
        }
        volume {
          name = "web"
          secret {
            secret_name = kubernetes_secret.ci_server_cert[0].metadata[0].name
            items {
              key  = "web-config.yaml"
              path = "web-config.yaml"
            }
          }
        }
        volume {
          name = "root"
          host_path {
            path = "/"
          }
        }
      }
    }
  }
}

resource "kubernetes_service" "node_exporter" {
  count = local.is_ci ? 1 : 0
  metadata {
    name      = "node-exporter-service"
    namespace = local.namespace
  }
  spec {
    selector = { app = "node-exporter" }
    port {
      name        = "https"
      port        = 9487
      target_port = 9487
    }
  }
}

#####################################################################
# Cluster-scraper agent Deployment: translates ci_cluster.json (apiserver,
# KSM, keda/karpenter). Mounts the CA at the KSM scrape path.
#####################################################################
resource "kubernetes_deployment_v1" "cluster_scraper" {
  count = local.is_ci ? 1 : 0
  metadata {
    name      = "cloudwatch-agent-cluster-scraper"
    namespace = local.namespace
    labels    = { app = "cloudwatch-agent-cluster-scraper" }
  }
  spec {
    replicas = 1
    selector {
      match_labels = { app = "cloudwatch-agent-cluster-scraper" }
    }
    template {
      metadata {
        labels = { app = "cloudwatch-agent-cluster-scraper" }
      }
      spec {
        service_account_name = kubernetes_service_account.cwagent.metadata[0].name
        image_pull_secrets {
          name = kubernetes_secret.ecr_pull.metadata[0].name
        }
        container {
          name              = "cloudwatch-agent"
          image             = "${local.cwagent_image_repo}:${var.cwagent_image_tag}"
          image_pull_policy = "Always"

          env {
            name  = "AWS_REGION"
            value = var.region
          }
          env {
            name  = "AWS_WEB_IDENTITY_TOKEN_FILE"
            value = "/var/run/secrets/aws/token"
          }
          env {
            name  = "AWS_ROLE_ARN"
            value = aws_iam_role.cwagent.arn
          }
          env {
            name  = "RUN_IN_CONTAINER"
            value = "True"
          }
          env {
            name  = "RUN_IN_AKS"
            value = "True"
          }
          env {
            name = "K8S_NODE_NAME"
            value_from {
              field_ref {
                field_path = "spec.nodeName"
              }
            }
          }

          volume_mount {
            name       = "aws-token"
            mount_path = "/var/run/secrets/aws"
            read_only  = true
          }
          volume_mount {
            name       = "cwagentconfig"
            mount_path = "/etc/cwagentconfig"
            read_only  = true
          }
          volume_mount {
            name       = "agent-cert"
            mount_path = "/etc/amazon-cloudwatch-observability-agent-cert"
            read_only  = true
          }
        }

        volume {
          name = "aws-token"
          projected {
            sources {
              service_account_token {
                audience           = "sts.amazonaws.com"
                expiration_seconds = 86400
                path               = "token"
              }
            }
          }
        }
        volume {
          name = "cwagentconfig"
          config_map {
            name = kubernetes_config_map.ci_cluster[0].metadata[0].name
          }
        }
        volume {
          name = "agent-cert"
          secret {
            secret_name = kubernetes_secret.ci_scrape_ca[0].metadata[0].name
          }
        }
      }
    }
  }

  depends_on = [
    kubernetes_cluster_role_binding.cwagent,
    aws_iam_role_policy_attachment.cwagent_server_policy,
  ]
}

#####################################################################
# keda/karpenter stub emitters + sample app (applied via kubectl).
#####################################################################
resource "null_resource" "ci_extra_manifests" {
  count = local.is_ci ? 1 : 0
  provisioner "local-exec" {
    command = <<-EOT
      kubectl --kubeconfig='${local_sensitive_file.kubeconfig.filename}' apply -f '${path.module}/../../../${var.test_dir}/resources/keda_karpenter.yaml'
    EOT
  }
  depends_on = [local_sensitive_file.kubeconfig]
}
