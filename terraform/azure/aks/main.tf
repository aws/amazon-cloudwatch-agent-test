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

  # ---- Which test suite this run is for ------------------------------------------------------
  # The same stack serves every AKS suite. var.test_dir selects the suite, and the flags below switch
  # the optional resources on or off with `count`.
  #   test_dir = ./test/azure/aks                   OTLP: default:otel agent plus a load generator
  #   test_dir = ./test/azure/aks/containerinsights OTel Container Insights (plus KEDA and Karpenter)
  #   test_dir = ./test/azure/aks/gpu               NVIDIA GPU metrics on an AKS-managed GPU pool
  #   test_dir = ./test/azure/aks/gpudriver         NVIDIA GPU metrics on driver-install GPU pools
  is_ci         = var.test_dir == "./test/azure/aks/containerinsights"
  is_gpu        = var.test_dir == "./test/azure/aks/gpu"
  is_gpu_driver = var.test_dir == "./test/azure/aks/gpudriver"
  # Suites with GPU node pools.
  has_gpu = local.is_gpu || local.is_gpu_driver
  # Suites that run OTel Container Insights, and so the cluster-scraper as well as the node agent.
  otel_ci = local.is_ci || local.has_gpu

  # Must match serviceName in test/azure/aks/aks_test.go -- the test derives the expected log stream
  # and the trace query filter from it.
  load_gen_service_name     = "aks-otlp-test-service"
  load_gen_duration_seconds = 180

  # ---- GPU node pools -------------------------------------------------------------------------
  # GPU suites add node pools next to the default system pool, one node per pool. The pool names are
  # not free-form: the Go tests find each pool's nodes by the name (kubernetes.azure.com/agentpool),
  # so they must match test/azure/aks/gpu and test/azure/aks/gpudriver.
  #
  # "managed" is AKS's --enable-managed-gpu flag (a preview feature):
  #   managed = true   AKS installs the NVIDIA driver, the device plugin and a host DCGM service
  #                    (metrics on :19400). The chart scrapes that host service (job "dcgm-host").
  #   managed = false  AKS installs only the driver. The device plugin and the chart's own
  #                    dcgm-exporter pod (metrics on :9400) are installed by this stack and the chart.
  #
  # "labels" is a space-separated list of key=value node labels. node-color=green is what the tests
  # filter on; nvidia.com/gpu.present=true is what the device plugin and the toolkit marker target.
  #
  # Per suite:
  #   gpu       gpumanaged: AKS installs driver, device plugin and DCGM on the host (:19400); the chart scrapes it.
  #   gpudriver gpuidle, gpuburn: AKS installs the driver only; the test installs the device plugin and the
  #             chart's dcgm-exporter pod (:9400) runs on both. gpuidle stays idle, gpuburn runs multi-gpu-burn
  #             (the EKS gpu_single and gpu_multi node groups).
  gpu_pools = local.is_gpu ? {
    gpumanaged = { managed = true, size = var.aks_gpu_vm_size, labels = "ci-test.example.com/node-color=green" }
    } : local.is_gpu_driver ? {
    gpuidle = { managed = false, size = var.aks_gpu_vm_size, labels = "ci-test.example.com/node-color=green nvidia.com/gpu.present=true" }
    gpuburn = { managed = false, size = var.aks_gpu_burn_vm_size, labels = "ci-test.example.com/node-color=green nvidia.com/gpu.present=true" }
  } : {}
  # Pool that runs the multi-gpu-burn load (the busy GPU). In the gpudriver suite the other pool,
  # gpuidle, deliberately runs nothing so the tests can check idle-GPU readings.
  gpu_burn_pool = local.is_gpu ? "gpumanaged" : "gpuburn"
  # Keeps ordinary pods off the (expensive) GPU nodes. The chart's agents tolerate every taint, and
  # the GPU workloads in this file tolerate this one explicitly.
  gpu_taint = "sku=gpu:NoSchedule"

  # The Helm chart to install. By default the stack clones var.helm_chart_repo at
  # var.helm_chart_branch. Set var.helm_chart_path to install a local checkout instead, for example
  # a chart branch that is not merged upstream yet.
  chart_dir = var.helm_chart_path != "" ? var.helm_chart_path : "./helm-charts/charts/amazon-cloudwatch-observability"

  # Chart values that differ per suite, keyed by test_dir. They are merged over the common values in
  # helm_values below. Both GPU suites turn classic Container Insights off and OTel Container
  # Insights plus the chart's DCGM exporter on.
  suite_helm_values = {
    "./test/azure/aks" = {
      "otelContainerInsights.enabled" = "false"
      "agent.config"                  = "default:otel"
    }
    "./test/azure/aks/containerinsights" = {
      "otelContainerInsights.enabled"      = "true"
      "otelContainerInsights.logs.enabled" = "true"
    }
    "./test/azure/aks/gpu" = {
      "containerInsights.enabled"     = "false"
      "otelContainerInsights.enabled" = "true"
      "dcgmExporter.enabled"          = "true"
    }
    "./test/azure/aks/gpudriver" = {
      "containerInsights.enabled"     = "false"
      "otelContainerInsights.enabled" = "true"
      "dcgmExporter.enabled"          = "true"
    }
  }

  # Final chart values, in increasing priority: common values, then the suite's values, then
  # var.helm_set_values (anything passed on the command line wins).
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
    local.suite_helm_values[var.test_dir],
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
# GPU node pools (gpu, gpudriver). Managed GPU is an AKS preview with no
# azurerm support, so the pools are added with the aks-preview CLI, one
# after the other (AKS rejects concurrent operations on a cluster).
# Deleting the cluster removes them, so there is no separate destroy step.
#
# Requirements on the machine running terraform:
#   - the Azure CLI (az), logged in or with ARM_* credentials in the environment
#   - quota for the GPU VM size in the region; for managed GPU, the subscription feature
#     ManagedGPUExperiencePreview must also be registered
# The aks-preview extension is installed or upgraded by the command itself.
#####################################################################
resource "null_resource" "gpu_node_pool" {
  count = local.has_gpu ? 1 : 0

  triggers = { cluster_id = azurerm_kubernetes_cluster.cwagent.id }

  provisioner "local-exec" {
    command     = <<-EOT
      set -euo pipefail
      az extension add --name aks-preview --upgrade --yes --only-show-errors
      # One az call per pool in local.gpu_pools, rendered by the template loop below. The
      # $${ARM_SUBSCRIPTION_ID:+...} part is bash, not Terraform: it adds --subscription only when
      # that variable is set.
      %{~for name, pool in local.gpu_pools~}
      az aks nodepool add $${ARM_SUBSCRIPTION_ID:+--subscription "$ARM_SUBSCRIPTION_ID"} \
        --resource-group '${var.azure_resource_group}' \
        --cluster-name '${azurerm_kubernetes_cluster.cwagent.name}' \
        --name ${name} \
        --node-count 1 \
        --node-vm-size '${pool.size}' \
        --vnet-subnet-id '${data.azurerm_subnet.selected.id}' \
        --node-taints '${local.gpu_taint}' \
        --labels ${pool.labels} \
        --enable-managed-gpu=${pool.managed} \
        --only-show-errors --output none
      %{~endfor~}
    EOT
    interpreter = ["/bin/bash", "-c"]
  }
}

# Driver-install pools (gpudriver) get no device plugin from AKS, so nothing would advertise the
# nvidia.com/gpu resource that the gpu-burn pod requests. Install NVIDIA's plugin chart. Its default
# node affinity matches the nvidia.com/gpu.present label set on these pools, as on the EKS GPU node
# groups. The tolerations let it run on the tainted GPU nodes. Managed-GPU pools do not need this,
# because AKS runs its own plugin there.
resource "helm_release" "nvidia_device_plugin" {
  count      = local.is_gpu_driver ? 1 : 0
  name       = "nvidia-device-plugin"
  repository = "https://nvidia.github.io/k8s-device-plugin"
  chart      = "nvidia-device-plugin"
  namespace  = "kube-system"

  set = [
    { name = "tolerations[0].key", value = "sku" },
    { name = "tolerations[0].operator", value = "Exists" },
    { name = "tolerations[0].effect", value = "NoSchedule" },
    { name = "tolerations[1].key", value = "nvidia.com/gpu" },
    { name = "tolerations[1].operator", value = "Exists" },
    { name = "tolerations[1].effect", value = "NoSchedule" },
  ]

  depends_on = [null_resource.gpu_node_pool]
}

# Same toolkit-ready marker as terraform/eks/daemon/otel-gpu, on the driver-install GPU nodes.
# NVIDIA components can gate on the node file /run/nvidia/validations/toolkit-ready, which the NVIDIA
# GPU Operator normally creates. This stack has no GPU Operator, so a privileged init container
# creates the file once per node and a pause container keeps the pod alive. Bidirectional mount
# propagation makes the file visible to other pods on the node. This copies the EKS stack
# (terraform/eks/daemon/otel-gpu); the file is a no-op for components that do not look for it.
resource "kubernetes_daemon_set_v1" "nvidia_toolkit_ready" {
  count = local.is_gpu_driver ? 1 : 0
  metadata {
    name      = "nvidia-toolkit-ready"
    namespace = "kube-system"
  }
  spec {
    selector {
      match_labels = { app = "nvidia-toolkit-ready" }
    }
    template {
      metadata {
        labels = { app = "nvidia-toolkit-ready" }
      }
      spec {
        node_selector = { "nvidia.com/gpu.present" = "true" }
        toleration {
          key      = "sku"
          operator = "Exists"
          effect   = "NoSchedule"
        }
        toleration {
          key      = "nvidia.com/gpu"
          operator = "Exists"
          effect   = "NoSchedule"
        }
        init_container {
          name    = "create-marker"
          image   = "busybox:stable"
          command = ["sh", "-c", "mkdir -p /run/nvidia/validations && touch /run/nvidia/validations/toolkit-ready"]
          security_context {
            privileged = true
          }
          volume_mount {
            name              = "run-nvidia"
            mount_path        = "/run/nvidia"
            mount_propagation = "Bidirectional"
          }
        }
        container {
          name  = "pause"
          image = "registry.k8s.io/pause:3.9"
        }
        volume {
          name = "run-nvidia"
          host_path {
            path = "/run/nvidia"
          }
        }
      }
    }
  }
  depends_on = [null_resource.gpu_node_pool]
}

#####################################################################
# Helm chart install
#####################################################################
data "external" "clone_helm_chart" {
  count = var.helm_chart_path == "" ? 1 : 0
  program = ["bash", "-c", <<-EOT
    rm -rf ./helm-charts
    git clone -b ${var.helm_chart_branch} ${var.helm_chart_repo} ./helm-charts
    echo '{"status":"ready"}'
  EOT
  ]
}

resource "helm_release" "aws_observability" {
  name      = "amazon-cloudwatch-observability"
  chart     = local.chart_dir
  namespace = kubernetes_namespace.cwagent.metadata[0].name

  set = [for name, value in local.helm_values : { name = name, value = value }]

  depends_on = [
    data.external.clone_helm_chart,
    aws_iam_role_policy_attachment.cwagent_server_policy,
    null_resource.gpu_node_pool,
    helm_release.nvidia_device_plugin,
    kubernetes_daemon_set_v1.nvidia_toolkit_ready,
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
      %{~if local.otel_ci~}
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
      %{~if local.otel_ci~}
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
# gpu-burn on gpumanaged (gpu) or gpuburn (gpudriver). Name, labels and
# container match terraform/eks/daemon/otel-gpu so the same checks apply.
# It keeps one GPU busy for an hour (the "3600" argument is seconds), longer than any test run, so
# the tests see non-zero utilisation. The pod label pod-color=magenta is what the tests look for on
# the driver-install path; on the managed path they assert that pod attribution is absent, because
# host DCGM has no pod mapping.
#####################################################################
resource "kubernetes_deployment_v1" "gpu_burn" {
  count = local.has_gpu ? 1 : 0
  metadata {
    name      = "multi-gpu-burn"
    namespace = "default"
  }
  spec {
    replicas = 1
    selector {
      match_labels = { app = "multi-gpu-burn" }
    }
    template {
      metadata {
        labels = {
          app                             = "multi-gpu-burn"
          "ci-test.example.com/pod-color" = "magenta"
        }
      }
      spec {
        node_selector = { "kubernetes.azure.com/agentpool" = local.gpu_burn_pool }
        toleration {
          key      = "sku"
          operator = "Exists"
          effect   = "NoSchedule"
        }
        toleration {
          key      = "nvidia.com/gpu"
          operator = "Exists"
          effect   = "NoSchedule"
        }
        container {
          name  = "gpu-burn"
          image = "chrstnhntschl/gpu_burn:latest"
          args  = ["3600"]
          resources {
            limits = { "nvidia.com/gpu" = "1" }
          }
        }
      }
    }
  }
  # Readiness is checked in gpu_workload_ready, so a stuck pod still leaves diagnostics and test results.
  wait_for_rollout = false
  depends_on       = [null_resource.gpu_node_pool]
}

# Waits for gpu-burn (and, for gpudriver, the chart's dcgm-exporter pods), then for GPU metrics to reach
# CloudWatch, as terraform/eks/daemon/otel-gpu does. Does not fail the apply: a missing workload or
# exporter is reported by the tests after the diagnostics.
# The final "sleep 360" is a fixed wait, not a check: it gives the agent time to scrape, process and
# publish a few intervals of GPU metrics, so the tests query CloudWatch after the data exists.
resource "null_resource" "gpu_workload_ready" {
  count = local.has_gpu ? 1 : 0
  provisioner "local-exec" {
    command = <<-EOT
      kubectl --kubeconfig='${local_sensitive_file.kubeconfig.filename}' -n default rollout status deployment/multi-gpu-burn --timeout=900s \
        || echo "multi-gpu-burn not ready; continuing to diagnostics and tests"
      %{~if local.is_gpu_driver~}
      for i in $(seq 1 30); do
        ${local.kubectl} get pod -l app.kubernetes.io/name=dcgm-exporter -o name | grep -q . && break
        sleep 20
      done
      ${local.kubectl} wait pod -l app.kubernetes.io/name=dcgm-exporter --for=condition=Ready --timeout=600s \
        || echo "dcgm-exporter pods not ready; continuing to diagnostics and tests"
      %{~endif~}
      sleep 360
    EOT
  }
  depends_on = [null_resource.restart_pods, kubernetes_deployment_v1.gpu_burn]
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
      %{~if local.otel_ci~}
      ${local.kubectl} logs deployment/cloudwatch-agent-cluster-scraper --tail=200 --prefix || true
      %{~endif~}
      # GPU state: which pool each node belongs to, whether AKS runs host DCGM on it, and where the
      # exporter, device plugin and gpu-burn pods landed. Explains a missing metric after destroy.
      %{~if local.has_gpu~}
      kubectl --kubeconfig='${local_sensitive_file.kubeconfig.filename}' get nodes \
        -L kubernetes.azure.com/agentpool,accelerator,kubernetes.azure.com/dcgm-exporter,nvidia.com/gpu.present || true
      kubectl --kubeconfig='${local_sensitive_file.kubeconfig.filename}' get pods -A -o wide \
        -l 'app.kubernetes.io/name in (dcgm-exporter,nvidia-device-plugin)' || true
      kubectl --kubeconfig='${local_sensitive_file.kubeconfig.filename}' get pods -n default -o wide -l app=multi-gpu-burn || true
      %{~endif~}
    EOT
  }

  depends_on = [null_resource.restart_pods, kubernetes_job_v1.otlp_load, null_resource.gpu_workload_ready]
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
        -azureVMSize=${local.has_gpu ? var.aks_gpu_vm_size : var.aks_node_vm_size} \
        -azureResourceGroup=${azurerm_kubernetes_cluster.cwagent.node_resource_group} \
        -v
    EOT

    environment = {
      AWS_REGION = var.region
      KUBECONFIG = abspath(local_sensitive_file.kubeconfig.filename)
    }
  }

  depends_on = [
    null_resource.restart_pods,
    kubernetes_job_v1.otlp_load,
    null_resource.ci_extra_manifests,
    null_resource.gpu_workload_ready,
    null_resource.agent_diagnostics,
  ]
}
