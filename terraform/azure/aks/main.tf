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
  kubectl              = "kubectl --kubeconfig='${local_sensitive_file.kubeconfig.filename}' -n ${local.namespace}"

  is_ci = var.test_dir == "./test/azure/aks/containerinsights"

  # Must match serviceName in test/azure/aks/aks_test.go -- the test derives the expected log stream
  # and the trace query filter from it.
  load_gen_service_name     = "aks-otlp-test-service"
  load_gen_duration_seconds = 180

  helm_values = merge(
    {
      "clusterName"                = azurerm_kubernetes_cluster.cwagent.name
      "region"                     = var.region
      "k8sMode"                    = "AKS"
      "roleArn"                    = aws_iam_role.cwagent.arn
      "applicationSignals.enabled" = "false"
      "containerInsights.enabled"  = "false"
      "containerLogs.enabled"      = "false"
    },
    local.is_ci ? {
      "otelContainerInsights.enabled"      = "true"
      "otelContainerInsights.logs.enabled" = "true"
      } : {
      "otelContainerInsights.enabled" = "false"
      "agent.config"                  = "default:otel"
    },
    var.helm_set_values,
  )
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

resource "aws_iam_role_policy_attachment" "cwagent_server_policy" {
  role       = aws_iam_role.cwagent.name
  policy_arn = "arn:aws:iam::aws:policy/CloudWatchAgentServerPolicy"
}

#####################################################################
# ECR pull secret: AKS nodes cannot pull the integration-test image from
# private ECR on their own, and the chart has no imagePullSecrets value.
#####################################################################
locals {
  cwagent_image_repo = replace(var.cwagent_image_repo, "/\\.ecr\\.[a-z0-9-]+\\./", ".ecr.${var.ecr_region}.")
}

data "aws_ecr_authorization_token" "ecr" {
  provider = aws.ecr
}

resource "kubernetes_namespace" "cwagent" {
  metadata {
    name = local.namespace
  }
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

#####################################################################
# Kubeconfig for the kubectl steps below
#####################################################################
resource "local_sensitive_file" "kubeconfig" {
  content         = azurerm_kubernetes_cluster.cwagent.kube_config_raw
  filename        = "${path.module}/kubeconfig"
  file_permission = "0600"
}

#####################################################################
# Helm chart install
#####################################################################
data "external" "clone_helm_chart" {
  program = ["bash", "-c", <<-EOT
    rm -rf ./helm-charts
    git clone -b ${var.helm_chart_branch} https://github.com/aws-observability/helm-charts.git ./helm-charts
    echo '{"status":"ready"}'
  EOT
  ]
}

resource "helm_release" "aws_observability" {
  name      = "amazon-cloudwatch-observability"
  chart     = "./helm-charts/charts/amazon-cloudwatch-observability"
  namespace = kubernetes_namespace.cwagent.metadata[0].name

  set = [for name, value in local.helm_values : { name = name, value = value }]

  depends_on = [
    data.external.clone_helm_chart,
    aws_iam_role_policy_attachment.cwagent_server_policy,
  ]
}

#####################################################################
# Point the chart's agents at the image under test
#####################################################################
resource "null_resource" "update_image" {
  depends_on = [helm_release.aws_observability, kubernetes_secret.ecr_pull]
  triggers   = { timestamp = timestamp() }
  provisioner "local-exec" {
    command = <<-EOT
      sleep 30
      ${local.kubectl} patch serviceaccount ${local.service_account_name} \
        -p '{"imagePullSecrets":[{"name":"${kubernetes_secret.ecr_pull.metadata[0].name}"}]}'
      ${local.kubectl} patch AmazonCloudWatchAgent cloudwatch-agent --type='json' \
        -p='[{"op": "replace", "path": "/spec/image", "value": "${local.cwagent_image_repo}:${var.cwagent_image_tag}"}]'
      %{~if local.is_ci~}
      ${local.kubectl} patch AmazonCloudWatchAgent cloudwatch-agent-cluster-scraper --type='json' \
        -p='[{"op": "replace", "path": "/spec/image", "value": "${local.cwagent_image_repo}:${var.cwagent_image_tag}"}]'
      %{~endif~}
      sleep 10
    EOT
  }
}

resource "null_resource" "restart_pods" {
  depends_on = [null_resource.update_image]
  triggers   = { timestamp = timestamp() }
  provisioner "local-exec" {
    command = <<-EOT
      ${local.kubectl} rollout restart daemonset/cloudwatch-agent
      ${local.kubectl} rollout status daemonset/cloudwatch-agent --timeout=300s
      %{~if local.is_ci~}
      ${local.kubectl} rollout restart deployment/cloudwatch-agent-cluster-scraper
      ${local.kubectl} rollout status deployment/cloudwatch-agent-cluster-scraper --timeout=300s
      %{~endif~}
    EOT
  }
}

#####################################################################
# Load generator (otlp): pushes OTLP to the agent's host port for 3 min
#####################################################################
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

  depends_on = [null_resource.restart_pods]
}

#####################################################################
# keda/karpenter stub emitters + sample app (containerinsights)
#####################################################################
resource "null_resource" "ci_extra_manifests" {
  count = local.is_ci ? 1 : 0
  provisioner "local-exec" {
    command = <<-EOT
      kubectl --kubeconfig='${local_sensitive_file.kubeconfig.filename}' apply -f '${path.module}/../../../${var.test_dir}/resources/keda_karpenter.yaml'
    EOT
  }
}

#####################################################################
# Diagnostics: surface agent pod state and logs in the job output so
# delivery failures are debuggable after the cluster is destroyed.
#####################################################################
resource "null_resource" "agent_diagnostics" {
  provisioner "local-exec" {
    command = <<-EOT
      ${local.kubectl} get pods -o wide || true
      ${local.kubectl} logs daemonset/cloudwatch-agent --tail=200 --prefix || true
      %{~if local.is_ci~}
      ${local.kubectl} logs deployment/cloudwatch-agent-cluster-scraper --tail=200 --prefix || true
      %{~endif~}
    EOT
  }

  depends_on = [null_resource.restart_pods, kubernetes_job_v1.otlp_load]
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

  depends_on = [
    null_resource.restart_pods,
    kubernetes_job_v1.otlp_load,
    null_resource.ci_extra_manifests,
    null_resource.agent_diagnostics,
  ]
}
