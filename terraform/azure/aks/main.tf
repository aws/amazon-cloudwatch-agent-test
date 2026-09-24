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
# Install the agent via the amazon-cloudwatch-observability Helm chart,
# the same path the scripts/azure/setup.sh onboarding flow uses. The
# operator the chart installs reconciles an AmazonCloudWatchAgent CR into
# the DaemonSet (default:otel) and a cluster-scraper Deployment (default).
#####################################################################
resource "kubernetes_namespace" "cwagent" {
  metadata {
    name = local.namespace
  }
}

# Kubeconfig for the kubectl steps below (helm/kubernetes providers auth in-memory from kube_config).
resource "local_sensitive_file" "kubeconfig" {
  content         = azurerm_kubernetes_cluster.cwagent.kube_config_raw
  filename        = "${path.module}/kubeconfig"
  file_permission = "0600"
}

# ECR pull secret so AKS nodes can pull the CWA image from AWS ECR. AKS nodes have no AWS identity of
# their own (unlike EKS node IAM), so the agent DaemonSet/scraper pods need this secret on their service
# account. The 12h auth token is fetched here with the runner's AWS credentials rather than passed in as
# a variable, which cannot survive the workflow's shell quoting. The integration-test image is published
# to us-west-2 only, while the job's CloudWatch region may differ -- pin the registry host to the ECR region.
locals {
  cwagent_image_repo = replace(var.cwagent_image_repo, "/\\.ecr\\.[a-z0-9-]+\\./", ".ecr.${var.ecr_region}.")
  cwagent_image      = "${local.cwagent_image_repo}:${var.cwagent_image_tag}"
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

# Install from a git checkout of the chart (matching the EKS suites) so a specific chart branch can be
# pinned in CI, rather than the published repo the onboarding script uses.
data "external" "clone_helm_chart" {
  program = ["bash", "-c", <<-EOT
    rm -rf ${path.module}/helm-charts
    git clone -q -b ${var.helm_chart_branch} https://github.com/aws-observability/helm-charts.git ${path.module}/helm-charts
    echo '{"status":"ready"}'
  EOT
  ]
}

# Values mirror scripts/azure/setup.sh: k8sMode=AKS + roleArn wire the projected-token web-identity path,
# container insights runs over the OTLP pipeline, and both agents are listed in full (--set replaces a
# whole list element, so omitting agents[1] would drop the cluster scraper).
resource "helm_release" "aws_observability" {
  name             = "amazon-cloudwatch-observability"
  chart            = "${path.module}/helm-charts/charts/amazon-cloudwatch-observability"
  namespace        = kubernetes_namespace.cwagent.metadata[0].name
  create_namespace = false

  set = [
    { name = "clusterName", value = azurerm_kubernetes_cluster.cwagent.name },
    { name = "region", value = var.region },
    { name = "k8sMode", value = "AKS" },
    { name = "roleArn", value = aws_iam_role.cwagent.arn },
    { name = "containerInsights.enabled", value = "false" },
    { name = "containerLogs.enabled", value = "false" },
    { name = "otelContainerInsights.enabled", value = "true" },
    { name = "otelContainerInsights.logs.enabled", value = "true" },
    { name = "agents[0].name", value = "cloudwatch-agent", type = "string" },
    { name = "agents[0].config", value = "default:otel", type = "string" },
    { name = "agents[1].name", value = "cloudwatch-agent-cluster-scraper", type = "string" },
    { name = "agents[1].mode", value = "deployment", type = "string" },
    { name = "agents[1].config", value = "default", type = "string" },
  ]

  depends_on = [
    data.external.clone_helm_chart,
    kubernetes_namespace.cwagent,
    aws_iam_role_policy_attachment.cwagent_server_policy,
  ]
}

# Attach the ECR pull secret to the chart-created agent service account (both the DaemonSet and the
# cluster-scraper run under it), then point both AmazonCloudWatchAgent CRs at the runner-built image.
# The chart exposes no value for either, so this is done with kubectl after install, as the EKS suites do
# for the image. Restarting after the patches lets the recreated pods pull the private image with the secret.
resource "null_resource" "configure_agent" {
  depends_on = [helm_release.aws_observability, kubernetes_secret.ecr_pull]
  triggers   = { image = local.cwagent_image }

  provisioner "local-exec" {
    environment = { KUBECONFIG = local_sensitive_file.kubeconfig.filename }
    command     = <<-EOT
      set -euo pipefail
      kubectl -n ${local.namespace} patch serviceaccount ${local.service_account_name} \
        -p '{"imagePullSecrets":[{"name":"${kubernetes_secret.ecr_pull.metadata[0].name}"}]}'
      for cr in cloudwatch-agent cloudwatch-agent-cluster-scraper; do
        kubectl -n ${local.namespace} patch AmazonCloudWatchAgent "$cr" --type=json \
          -p="[{\"op\":\"replace\",\"path\":\"/spec/image\",\"value\":\"${local.cwagent_image}\"}]"
      done
      kubectl -n ${local.namespace} rollout restart daemonset/cloudwatch-agent
      kubectl -n ${local.namespace} rollout restart deployment/cloudwatch-agent-cluster-scraper
      kubectl -n ${local.namespace} rollout status daemonset/cloudwatch-agent --timeout=180s
      kubectl -n ${local.namespace} rollout status deployment/cloudwatch-agent-cluster-scraper --timeout=180s
    EOT
  }
}

#####################################################################
# Load generator: pushes OTLP to localhost:4318 for 3 min via hostNetwork
#####################################################################
# See otlp_load_generator.sh for what the payloads carry and why.
resource "kubernetes_job_v1" "otlp_load" {
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

  depends_on = [null_resource.configure_agent]
}

#####################################################################
# Diagnostics: surface agent pod state and logs in the job output so
# delivery failures are debuggable after the cluster is destroyed.
#####################################################################
resource "null_resource" "agent_diagnostics" {
  provisioner "local-exec" {
    command = <<-EOT
      kubectl --kubeconfig='${local_sensitive_file.kubeconfig.filename}' get pods -n amazon-cloudwatch -o wide || true
      kubectl --kubeconfig='${local_sensitive_file.kubeconfig.filename}' logs -n amazon-cloudwatch -l app.kubernetes.io/name=cloudwatch-agent --tail=200 --prefix || true
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
        -azureLocation=${var.azure_location} \
        -azureVMSize=${var.aks_node_vm_size} \
        -azureResourceGroup=${azurerm_kubernetes_cluster.cwagent.node_resource_group} \
        -v
    EOT

    environment = {
      AWS_REGION = var.region
    }
  }

  depends_on = [kubernetes_job_v1.otlp_load, null_resource.agent_diagnostics]
}
