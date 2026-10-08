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

  # test_mode == "containerinsights" swaps the raw default:otel DaemonSet + OTLP load generator for a
  # Helm chart install with OTel Container Insights, the way AKS customers deploy it. The chart owns the
  # agent ServiceAccount (named cloudwatch-agent, matching the IAM trust above), RBAC and workloads.
  is_ci = var.test_mode == "containerinsights"

  # Must match serviceName in test/azure/aks/aks_test.go -- the test derives the expected log stream
  # and the trace query filter from it.
  load_gen_service_name     = "aks-otlp-test-service"
  load_gen_duration_seconds = 180
}

# Couple test_mode and test_dir so a caller can't run one suite against the other's topology
# (e.g. test_mode=containerinsights with the default:otel test_dir). Fails the plan on a mismatch.
resource "terraform_data" "validate_test_mode_dir" {
  lifecycle {
    precondition {
      condition = (
        (var.test_mode == "otlp" && var.test_dir == "./test/azure/aks") ||
        (var.test_mode == "containerinsights" && var.test_dir == "./test/azure/aks/containerinsights")
      )
      error_message = "test_dir must match test_mode: \"otlp\" -> \"./test/azure/aks\", \"containerinsights\" -> \"./test/azure/aks/containerinsights\"."
    }
  }
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
# Kubernetes resources. The namespace and ECR pull secret are shared by
# both modes; the ServiceAccount, RBAC and DaemonSet below are OTLP-mode
# only (in containerinsights mode the Helm chart creates them).
#####################################################################
resource "kubernetes_namespace" "cwagent" {
  metadata {
    name = local.namespace
  }
}

resource "kubernetes_service_account" "cwagent" {
  count = local.is_ci ? 0 : 1
  metadata {
    name      = local.service_account_name
    namespace = kubernetes_namespace.cwagent.metadata[0].name
  }
}

resource "kubernetes_cluster_role" "cwagent" {
  count = local.is_ci ? 0 : 1
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
  count = local.is_ci ? 0 : 1
  metadata {
    name = "cwa-aks-integ-${module.common.testing_id}"
  }
  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "ClusterRole"
    name      = kubernetes_cluster_role.cwagent[0].metadata[0].name
  }
  subject {
    kind      = "ServiceAccount"
    name      = kubernetes_service_account.cwagent[0].metadata[0].name
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
  count = local.is_ci ? 0 : 1
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
        service_account_name = kubernetes_service_account.cwagent[0].metadata[0].name
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
          # Uses the agent's built-in default:otel config.
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
          args = [templatefile("${path.module}/../../otlp_load_generator.sh", {
            prefix           = "aks"
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
      kubectl --kubeconfig='${local_sensitive_file.kubeconfig.filename}' logs -n amazon-cloudwatch ds/cloudwatch-agent --tail=200 --prefix || true
      kubectl --kubeconfig='${local_sensitive_file.kubeconfig.filename}' logs -n amazon-cloudwatch deploy/cloudwatch-agent-cluster-scraper --tail=200 --prefix || true
    EOT
  }

  depends_on = [kubernetes_daemon_set_v1.cwagent, kubernetes_job_v1.otlp_load, null_resource.ci_helm_chart]
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
        -azureLocation=${var.azure_location} \
        -azureVMSize=${var.aks_node_vm_size} \
        -azureResourceGroup=${azurerm_kubernetes_cluster.cwagent.node_resource_group} \
        -v
    EOT

    environment = {
      AWS_REGION = var.region
    }
  }

  depends_on = [kubernetes_daemon_set_v1.cwagent, kubernetes_job_v1.otlp_load, null_resource.ci_helm_chart, null_resource.ci_extra_manifests, null_resource.agent_diagnostics]
}

#####################################################################
# Container Insights topology (test_mode == "containerinsights"): the
# amazon-cloudwatch-observability Helm chart with OTel Container Insights,
# installed with the same values scripts/azure/setup.sh uses for AKS
# customers, plus the agent image under test. The chart deploys the node
# DaemonSet, the cluster-scraper Deployment, node-exporter and
# kube-state-metrics, and renders their collector config itself.
#####################################################################
locals {
  cwagent_image_domain     = split("/", local.cwagent_image_repo)[0]
  cwagent_image_repository = join("/", slice(split("/", local.cwagent_image_repo), 1, length(split("/", local.cwagent_image_repo))))
  helm_charts_dir          = "${path.module}/helm-charts"
}

resource "null_resource" "ci_helm_chart" {
  count = local.is_ci ? 1 : 0

  triggers = {
    kubeconfig = local_sensitive_file.kubeconfig.filename
    namespace  = local.namespace
  }

  # The chart's agent ServiceAccount has no image pull secret setting, and the image under test is in a
  # private ECR, so attach the ECR pull secret to the ServiceAccount after install and restart the agent
  # workloads the operator created so their pods pick it up.
  provisioner "local-exec" {
    command     = <<-EOT
      set -euo pipefail
      KC='${local_sensitive_file.kubeconfig.filename}'
      rm -rf '${local.helm_charts_dir}'
      git clone https://github.com/aws-observability/helm-charts.git '${local.helm_charts_dir}'
      git -C '${local.helm_charts_dir}' checkout '${var.helm_charts_branch}'
      helm --kubeconfig "$KC" upgrade --install amazon-cloudwatch-observability \
        '${local.helm_charts_dir}/charts/amazon-cloudwatch-observability' \
        --namespace '${local.namespace}' \
        --set k8sMode=AKS \
        --set roleArn='${aws_iam_role.cwagent.arn}' \
        --set region='${var.region}' \
        --set clusterName='${azurerm_kubernetes_cluster.cwagent.name}' \
        --set containerInsights.enabled=false \
        --set containerLogs.enabled=false \
        --set otelContainerInsights.enabled=true \
        --set otelContainerInsights.logs.enabled=true \
        --set-string 'agents[0].name=cloudwatch-agent' \
        --set-string 'agents[0].config=default:otel' \
        --set-string 'agents[1].name=cloudwatch-agent-cluster-scraper' \
        --set-string 'agents[1].mode=deployment' \
        --set-string 'agents[1].config=default' \
        --set-string agent.image.repositoryDomainMap.public='${local.cwagent_image_domain}' \
        --set-string agent.image.repository='${local.cwagent_image_repository}' \
        --set-string agent.image.tag='${var.cwagent_image_tag}'
      kubectl --kubeconfig "$KC" -n '${local.namespace}' patch serviceaccount cloudwatch-agent \
        -p '{"imagePullSecrets":[{"name":"${kubernetes_secret.ecr_pull.metadata[0].name}"}]}'
      for w in ds/cloudwatch-agent deploy/cloudwatch-agent-cluster-scraper; do
        for i in $(seq 1 60); do
          kubectl --kubeconfig "$KC" -n '${local.namespace}' get "$w" >/dev/null 2>&1 && break
          sleep 5
        done
        kubectl --kubeconfig "$KC" -n '${local.namespace}' rollout restart "$w"
        kubectl --kubeconfig "$KC" -n '${local.namespace}' rollout status "$w" --timeout=10m
      done
    EOT
    interpreter = ["/bin/bash", "-c"]
  }

  # Uninstall while the operator still runs, so it can clear its CR finalizers before the namespace is deleted.
  provisioner "local-exec" {
    when    = destroy
    command = "helm --kubeconfig '${self.triggers.kubeconfig}' uninstall amazon-cloudwatch-observability --namespace '${self.triggers.namespace}' --wait || true"
  }

  depends_on = [
    kubernetes_secret.ecr_pull,
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
