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

  # Terraform drives the cluster over the public API server, so restrict it to the runner that
  # created it. Left unrestricted when runner_ip is empty (local runs without a known egress IP).
  api_server_access_profile {
    authorized_ip_ranges = var.runner_ip != "" ? [var.runner_ip] : []
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

data "aws_iam_policy_document" "cwagent_permissions" {
  statement {
    effect = "Allow"
    actions = [
      "cloudwatch:PutMetricData",
      "cloudwatch:ListMetrics",
      "cloudwatch:GetMetricData",
      "logs:PutLogEvents",
      "logs:CreateLogGroup",
      "logs:CreateLogStream",
      "logs:DescribeLogGroups",
      "logs:DescribeLogStreams",
      "logs:GetLogEvents",
      "logs:StartQuery",
      "logs:GetQueryResults",
      "xray:PutSpans",
      "xray:PutTraceSegments",
      "xray:PutTelemetryRecords",
    ]
    resources = ["*"]
  }

}

resource "aws_iam_role_policy" "cwagent" {
  name   = "cwa-aks-integ-policy-${module.common.testing_id}"
  role   = aws_iam_role.cwagent.id
  policy = data.aws_iam_policy_document.cwagent_permissions.json
}

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
    namespace = local.namespace
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
    namespace = local.namespace
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
    namespace = local.namespace
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
          env {
            name  = "USE_DEFAULT_CONFIG"
            value = "otel"
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
      }
    }
  }

  depends_on = [
    kubernetes_cluster_role_binding.cwagent,
    aws_iam_role_policy.cwagent,
  ]
}

#####################################################################
# Load generator: pushes OTLP to localhost:4318 for 3 min via hostNetwork
#####################################################################
# The payloads carry k8s.cluster.name and k8s.namespace.name resource attributes so the
# agent's k8s logs-routing template produces a deterministic per-cluster destination
# (/aws/cwagent/<cluster>/otlp, stream amazon-cloudwatch/<service>); resourcedetection
# only overrides keys it detects (e.g. host.id), so these pass through intact.
resource "kubernetes_job_v1" "otlp_load" {
  metadata {
    name      = "otlp-load-generator"
    namespace = local.namespace
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

  depends_on = [kubernetes_job_v1.otlp_load]
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

  depends_on = [kubernetes_job_v1.otlp_load, null_resource.agent_diagnostics]
}
