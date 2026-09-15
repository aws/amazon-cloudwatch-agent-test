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

  # Must match serviceName in test/gcp/gke/gke_test.go -- the test derives the expected log stream
  # and the trace query filter from it.
  load_gen_service_name     = "gke-otlp-test-service"
  load_gen_duration_seconds = 180
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
    name = "cwa-gke-integ-${module.common.testing_id}"
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
    name = "cwa-gke-integ-${module.common.testing_id}"
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

# ECR pull secret so GKE nodes can pull the CWA image from AWS ECR.
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
          # Explicit GKE signal so mode detection selects the GCP credential/region
          # path without depending on a metadata-server probe from the pod.
          env {
            name  = "RUN_IN_GKE"
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
    aws_iam_role_policy_attachment.cwagent_server_policy,
  ]
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

  depends_on = [kubernetes_daemon_set_v1.cwagent]
}

#####################################################################
# Diagnostics: surface agent pod state and logs in the job output so
# delivery failures are debuggable after the cluster is destroyed.
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
        -computeType=GKE \
        -region=${var.region} \
        -cwaCommitSha=${var.cwa_github_sha} \
        -gkeClusterName=${google_container_cluster.cwagent.name} \
        -v
    EOT

    environment = {
      AWS_REGION = var.region
    }
  }

  depends_on = [kubernetes_job_v1.otlp_load, null_resource.agent_diagnostics]
}
