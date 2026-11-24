// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "common" {
  source = "../../common"
}

#####################################################################
# GKE cluster (its OIDC issuer is what AWS trusts for cross-cloud web-identity)
#####################################################################
# GKE always serves the cluster's OIDC discovery document and projects service-account
# tokens; there is no issuer opt-in flag to set.
resource "google_container_cluster" "cwagent" {
  name               = "cwa-gke-integ-${module.common.testing_id}"
  location           = var.gcp_zone
  min_master_version = var.kubernetes_version
  network            = var.gcp_network_name
  subnetwork         = var.gcp_subnetwork_name

  initial_node_count = var.gke_node_count

  # The google provider defaults this to true, which would make terraform destroy fail.
  deletion_protection = false

  # Terraform drives the cluster over the public API server, so restrict it to the runner that created it.
  # runner_ip is required, so there is no path where this silently ends up open to all.
  master_authorized_networks_config {
    cidr_blocks {
      cidr_block = var.runner_ip
    }
  }

  node_config {
    machine_type = var.gke_node_machine_type
    disk_size_gb = 50
    oauth_scopes = ["https://www.googleapis.com/auth/cloud-platform"]

    # The standard suite pins its test workloads to this label, as the EKS stack does with its node
    # group (the EKS value is "blue"), and the tests read it back as a custom node label.
    labels = local.is_standard ? { "ci-test.example.com/node-color" = "blue" } : {}
  }
}

#####################################################################
# AWS IAM: trust GKE OIDC issuer for cross-cloud federation
#####################################################################
locals {
  # GKE serves the issuer at this deterministic URL; the cluster resource does not export it
  # as an attribute. Referencing the resource's name/location makes everything derived from
  # this URL wait for the cluster, so the discovery endpoint is live before it is read.
  gke_oidc_issuer_url = "https://container.googleapis.com/v1/projects/${var.gcp_project}/locations/${google_container_cluster.cwagent.location}/clusters/${google_container_cluster.cwagent.name}"
}

data "tls_certificate" "gke_oidc" {
  url = local.gke_oidc_issuer_url
}

resource "aws_iam_openid_connect_provider" "gke" {
  url             = local.gke_oidc_issuer_url
  client_id_list  = ["sts.amazonaws.com"]
  thumbprint_list = [data.tls_certificate.gke_oidc.certificates[0].sha1_fingerprint]
}

locals {
  gke_oidc_issuer_host = replace(local.gke_oidc_issuer_url, "https://", "")
  namespace            = "amazon-cloudwatch"
  service_account_name = "cloudwatch-agent"
  cwagent_role_name    = "cwa-gke-integ-role-${module.common.testing_id}"
}

data "aws_iam_policy_document" "cwagent_assume_role" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [aws_iam_openid_connect_provider.gke.arn]
    }

    condition {
      test     = "StringEquals"
      variable = "${local.gke_oidc_issuer_host}:sub"
      values   = ["system:serviceaccount:${local.namespace}:${local.service_account_name}"]
    }

    condition {
      test     = "StringEquals"
      variable = "${local.gke_oidc_issuer_host}:aud"
      values   = ["sts.amazonaws.com"]
    }
  }

}

resource "aws_iam_role" "cwagent" {
  name               = local.cwagent_role_name
  assume_role_policy = data.aws_iam_policy_document.cwagent_assume_role.json
}

# The agent's own writes come from the same AWS-managed policy customers are told to use, so a green run
# also proves that documented policy is sufficient over the GKE projected-token path.
resource "aws_iam_role_policy_attachment" "cwagent_server_policy" {
  role       = aws_iam_role.cwagent.name
  policy_arn = "arn:aws:iam::aws:policy/CloudWatchAgentServerPolicy"
}

# No inline policy: the GKE test binary runs on the runner under its own credentials, so this role needs
# agent writes only -- and CloudWatchAgentServerPolicy alone covers them, OTLP traces included.

#####################################################################
# ECR pull secret: GKE nodes cannot pull the integration-test image from
# private ECR on their own, and the chart has no imagePullSecrets value.
#####################################################################
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
# GKE has no ready-made kubeconfig attribute, so build a static one from the cluster
# endpoint and the caller's ADC bearer token (valid ~1h, longer than a run) -- kubectl
# then needs no gcloud auth plugin on the machine running terraform.
resource "local_sensitive_file" "kubeconfig" {
  content = yamlencode({
    apiVersion = "v1"
    kind       = "Config"
    clusters = [{
      name = google_container_cluster.cwagent.name
      cluster = {
        server                       = "https://${google_container_cluster.cwagent.endpoint}"
        "certificate-authority-data" = google_container_cluster.cwagent.master_auth[0].cluster_ca_certificate
      }
    }]
    users = [{
      name = "terraform"
      user = {
        token = data.google_client_config.current.access_token
      }
    }]
    contexts = [{
      name = google_container_cluster.cwagent.name
      context = {
        cluster = google_container_cluster.cwagent.name
        user    = "terraform"
      }
    }]
    "current-context" = google_container_cluster.cwagent.name
  })
  filename        = "${path.module}/kubeconfig"
  file_permission = "0600"
}

#####################################################################
# Helm chart install
#####################################################################
locals {
  kubectl = "kubectl --kubeconfig='${local_sensitive_file.kubeconfig.filename}' -n ${local.namespace}"

  # ---- Which test suite this run is for -------------------------------------------------------
  # The same stack serves every GKE suite. var.test_dir selects the suite, and the flags below switch
  # the optional resources on or off with `count`.
  #   test_dir = ./test/gcp/gke          OTLP: default:otel agent plus a load generator
  #   test_dir = ./test/gcp/gke/standard OTel Container Insights checked in depth, with test workloads
  is_standard = var.test_dir == "./test/gcp/gke/standard"

  # Must match serviceName in test/gcp/gke/gke_test.go -- the test derives the expected log stream
  # and the trace query filter from it.
  load_gen_service_name     = "gke-otlp-test-service"
  load_gen_duration_seconds = 180

  # Chart values that differ per suite, keyed by test_dir, merged over the common values below.
  # Application Signals and container logs stay off for every suite: Application Signals has no GKE
  # support in the chart, and the chart's fluent-bit has no GKE web-identity wiring.
  suite_helm_values = {
    "./test/gcp/gke" = {
      "agent.config"                  = "default:otel"
      "otelContainerInsights.enabled" = "false"
    }
    "./test/gcp/gke/standard" = {
      "otelContainerInsights.enabled"      = "true"
      "otelContainerInsights.logs.enabled" = "true"
    }
  }

  # Final chart values, in increasing priority: common values, then the suite's values, then
  # var.helm_set_values (anything passed on the command line wins).
  helm_values = merge(
    {
      "clusterName"                = google_container_cluster.cwagent.name
      "region"                     = var.region
      "k8sMode"                    = "GKE"
      "roleArn"                    = aws_iam_role.cwagent.arn
      "applicationSignals.enabled" = "false"
      "containerInsights.enabled"  = "false"
      "containerLogs.enabled"      = "false"
    },
    local.suite_helm_values[var.test_dir],
    var.helm_set_values,
  )
}

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
# Point the chart's agent at the image under test
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
    EOT
  }
}

#####################################################################
# Load generator: pushes OTLP to localhost:4318 for 3 min via hostNetwork
#####################################################################
# See otlp_load_generator.sh for what the payloads carry and why.
resource "kubernetes_job_v1" "otlp_load" {
  count = local.is_standard ? 0 : 1
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
            prefix           = "gke"
            service_name     = local.load_gen_service_name
            instance_id      = google_container_cluster.cwagent.name
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
# Test workloads for the standard suite. They copy terraform/eks/daemon/otel (and the AKS standard
# suite), so the kube-state-metrics workload metrics (statefulset, job, cronjob, replicaset,
# deployment) have data and the tests have known workloads to compare against. They land on the
# default pool, which carries the node-color=blue label in this suite.
#####################################################################
resource "kubernetes_deployment_v1" "nginx_test" {
  count = local.is_standard ? 1 : 0
  metadata {
    name      = "nginx-test"
    namespace = "default"
  }
  spec {
    replicas = 1
    selector { match_labels = { app = "nginx-test" } }
    template {
      metadata { labels = { app = "nginx-test" } }
      spec {
        container {
          name  = "nginx"
          image = "public.ecr.aws/nginx/nginx:latest"
          port { container_port = 80 }
        }
      }
    }
  }
  depends_on = [google_container_cluster.cwagent]
}

resource "kubernetes_service_v1" "ksm_statefulset_headless" {
  count = local.is_standard ? 1 : 0
  metadata {
    name      = "ksm-test-statefulset"
    namespace = "default"
  }
  spec {
    cluster_ip = "None"
    selector   = { app = "ksm-test-statefulset" }
    port {
      port = 80
      name = "placeholder"
    }
  }
  depends_on = [google_container_cluster.cwagent]
}

resource "kubernetes_stateful_set_v1" "ksm_statefulset" {
  count = local.is_standard ? 1 : 0
  metadata {
    name      = "ksm-test-statefulset"
    namespace = "default"
  }
  spec {
    replicas     = 1
    service_name = kubernetes_service_v1.ksm_statefulset_headless[0].metadata[0].name
    selector { match_labels = { app = "ksm-test-statefulset" } }
    template {
      metadata { labels = { app = "ksm-test-statefulset" } }
      spec {
        node_selector = { "ci-test.example.com/node-color" = "blue" }
        container {
          name  = "pause"
          image = "registry.k8s.io/pause:3.9"
          resources { requests = { cpu = "10m", memory = "16Mi" } }
        }
      }
    }
  }
}

resource "kubernetes_cron_job_v1" "ksm_cronjob" {
  count = local.is_standard ? 1 : 0
  metadata {
    name      = "ksm-test-cronjob"
    namespace = "default"
  }
  spec {
    schedule                      = "*/5 * * * *"
    successful_jobs_history_limit = 1
    failed_jobs_history_limit     = 1
    job_template {
      metadata {}
      spec {
        template {
          metadata { labels = { app = "ksm-test-cronjob" } }
          spec {
            node_selector  = { "ci-test.example.com/node-color" = "blue" }
            restart_policy = "Never"
            container {
              name    = "echo"
              image   = "busybox:1.36"
              command = ["echo", "ksm-test"]
              resources { requests = { cpu = "10m", memory = "16Mi" } }
            }
          }
        }
      }
    }
  }
  depends_on = [google_container_cluster.cwagent]
}

resource "kubernetes_job_v1" "ksm_job" {
  count = local.is_standard ? 1 : 0
  metadata {
    name      = "ksm-test-job"
    namespace = "default"
  }
  spec {
    ttl_seconds_after_finished = 86400
    template {
      metadata { labels = { app = "ksm-test-job" } }
      spec {
        node_selector  = { "ci-test.example.com/node-color" = "blue" }
        restart_policy = "Never"
        container {
          name    = "echo"
          image   = "busybox:1.36"
          command = ["echo", "ksm-test"]
          resources { requests = { cpu = "10m", memory = "16Mi" } }
        }
      }
    }
  }
  depends_on = [google_container_cluster.cwagent]
}

# The kubernetes provider has no ReplicaSet resource, so apply one with kubectl.
resource "null_resource" "ksm_replicaset" {
  count      = local.is_standard ? 1 : 0
  depends_on = [google_container_cluster.cwagent, local_sensitive_file.kubeconfig]
  provisioner "local-exec" {
    command = <<-EOT
      cat <<'EOF' | kubectl --kubeconfig='${local_sensitive_file.kubeconfig.filename}' apply -f -
      apiVersion: apps/v1
      kind: ReplicaSet
      metadata:
        name: ksm-test-replicaset
        namespace: default
      spec:
        replicas: 1
        selector:
          matchLabels:
            app: ksm-test-replicaset
        template:
          metadata:
            labels:
              app: ksm-test-replicaset
          spec:
            nodeSelector:
              ci-test.example.com/node-color: blue
            containers:
            - name: pause
              image: registry.k8s.io/pause:3.9
              resources:
                requests:
                  cpu: 10m
                  memory: 16Mi
      EOF
    EOT
  }
}

# Waits for the workloads, then 240 s for metrics to reach CloudWatch, as the EKS and AKS stacks do
# before go test. A workload that is not ready does not stop the apply, so diagnostics and test
# results still appear.
resource "null_resource" "standard_workload_ready" {
  count = local.is_standard ? 1 : 0
  provisioner "local-exec" {
    command = <<-EOT
      kubectl --kubeconfig='${local_sensitive_file.kubeconfig.filename}' -n default rollout status deployment/nginx-test --timeout=300s \
        || echo "nginx-test not ready; continuing to diagnostics and tests"
      kubectl --kubeconfig='${local_sensitive_file.kubeconfig.filename}' -n default rollout status statefulset/ksm-test-statefulset --timeout=300s \
        || echo "ksm-test-statefulset not ready; continuing to diagnostics and tests"
      kubectl --kubeconfig='${local_sensitive_file.kubeconfig.filename}' -n default wait --for=condition=complete job/ksm-test-job --timeout=300s \
        || echo "ksm-test-job not complete; continuing to diagnostics and tests"
      sleep 240
    EOT
  }
  depends_on = [
    null_resource.restart_pods,
    kubernetes_deployment_v1.nginx_test,
    kubernetes_stateful_set_v1.ksm_statefulset,
    kubernetes_cron_job_v1.ksm_cronjob,
    kubernetes_job_v1.ksm_job,
    null_resource.ksm_replicaset,
  ]
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
    EOT
  }

  depends_on = [kubernetes_job_v1.otlp_load, null_resource.standard_workload_ready]
}

#####################################################################
# Run Go integration test from the runner (validates CloudWatch)
#####################################################################
resource "null_resource" "integration_test" {
  provisioner "local-exec" {
    working_dir = "${path.module}/../../../"
    command     = <<-EOT
      go test -tags integration ${var.test_dir} -p 1 -timeout 30m \
        -computeType=GKE \
        -region=${var.region} \
        -cwaCommitSha=${var.cwa_github_sha} \
        -gkeClusterName=${google_container_cluster.cwagent.name} \
        -gcpProject=${var.gcp_project} \
        -gcpZone=${var.gcp_zone} \
        -v
    EOT

    environment = {
      AWS_REGION = var.region
      # The standard suite's TestKSM_NodeBucket_AZMatchesProviderID reads the cluster's nodes
      # directly (k8s_helpers_test.go's getGroundTruth), which needs a kubeconfig. Without this,
      # it falls back to ~/.kube/config, which does not exist on a CI runner (confirmed: run
      # 45377121910, 2026-11-24, "no such file or directory").
      KUBECONFIG = local_sensitive_file.kubeconfig.filename
    }
  }

  depends_on = [kubernetes_job_v1.otlp_load, null_resource.standard_workload_ready, null_resource.agent_diagnostics]
}
