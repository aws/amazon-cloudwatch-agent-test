# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
# SPDX-License-Identifier: MIT
#
# E2E harness for the Target Allocator per-node allocation strategy plus zero-step
# ServiceMonitor/PodMonitor CRDs (G1) and TA resilience to missing CRDs
# (G2). Compared with ../otel (the standard suite) this harness:
#   * installs a helm-charts checkout with otelPrometheusMonitor.enabled (and
#     otelContainerInsights on or off, see otel_container_insights_enabled), so the operator
#     creates the SM/PM CRDs itself (no separate prometheus-operator CRD install --
#     that is the whole point of G1),
#   * deploys CUSTOM operator and Target Allocator images carrying the per-node,
#     CRD-creation and CRD-watch code (the public images do not have it),
#   * relies on the chart's per-node allocation strategy for the per-node agent,
#   * applies a ServiceMonitor/PodMonitor workload with a target_node relabel,
#   * runs ./test/otel/pernode.

module "common" {
  source             = "../../../common"
  cwagent_image_repo = var.cwagent_image_repo
  cwagent_image_tag  = var.cwagent_image_tag
}

module "basic_components" {
  source = "../../../basic_components"
  region = var.region
}

locals {
  aws_eks = "aws eks --region ${var.region}"
}

resource "aws_eks_cluster" "this" {
  name     = "cwagent-eks-pernode-${module.common.testing_id}"
  role_arn = module.basic_components.role_arn
  version  = var.k8s_version
  vpc_config {
    subnet_ids         = module.basic_components.public_subnet_ids
    security_group_ids = [module.basic_components.security_group]
  }
}

# EKS Node Group -- >=2 nodes so per-node spread is meaningful.
resource "aws_eks_node_group" "this" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "cwagent-pernode-node-${module.common.testing_id}"
  node_role_arn   = aws_iam_role.node_role.arn
  subnet_ids      = module.basic_components.public_subnet_ids

  scaling_config {
    desired_size = var.node_count
    max_size     = var.node_count
    min_size     = var.node_count
  }

  ami_type       = var.ami_type
  capacity_type  = "ON_DEMAND"
  disk_size      = 20
  instance_types = [var.instance_type]

  depends_on = [
    aws_iam_role_policy_attachment.node_AmazonEC2ContainerRegistryReadOnly,
    aws_iam_role_policy_attachment.node_AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy,
  ]
}

# EKS Node IAM Role
resource "aws_iam_role" "node_role" {
  name = "cwagent-pernode-Worker-Role-${module.common.testing_id}"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ec2.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
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

# Pod Identity IAM Role
resource "aws_iam_role" "pod_identity_role" {
  name = "cwagent-pernode-pod-identity-${module.common.testing_id}"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "pods.eks.amazonaws.com" }
      Action    = ["sts:AssumeRole", "sts:TagSession"]
    }]
  })
}

resource "aws_iam_role_policy_attachment" "pod_identity_CloudWatchAgentServerPolicy" {
  policy_arn = "arn:aws:iam::aws:policy/CloudWatchAgentServerPolicy"
  role       = aws_iam_role.pod_identity_role.name
}

# --- EKS Addon: Pod Identity agent ---

resource "aws_eks_addon" "pod_identity_agent" {
  depends_on   = [aws_eks_node_group.this]
  cluster_name = aws_eks_cluster.this.name
  addon_name   = "eks-pod-identity-agent"
}

# --- Update kubeconfig ---

resource "null_resource" "kubectl" {
  depends_on = [aws_eks_cluster.this, aws_eks_node_group.this]
  provisioner "local-exec" {
    command = "${local.aws_eks} update-kubeconfig --name ${aws_eks_cluster.this.name}"
  }
}

# --- Clone the helm-charts checkout with SM/PM scraping (the operator creates the
#     SM/PM CRDs, G1). ---
# NOTE: no prometheus-operator CRD install step exists in this harness on
# purpose -- the SM/PM CRDs must come from the operator alone.

resource "null_resource" "clone_helm_chart" {
  count = var.local_chart_path == "" ? 1 : 0
  # Re-clone only when the source ref changes (not on every plan). A data source
  # must be side-effect free; cloning here (a resource) with an idempotent guard
  # avoids the previous rm -rf of a directory in the caller's cwd.
  triggers = {
    repo   = var.helm_chart_repo
    branch = var.helm_chart_branch
  }
  provisioner "local-exec" {
    command = <<-EOT
      set -e
      dest="${path.module}/helm-charts"
      if [ ! -d "$dest/.git" ]; then
        git clone --depth 1 -b ${var.helm_chart_branch} ${var.helm_chart_repo} "$dest"
      else
        git -C "$dest" fetch --depth 1 origin ${var.helm_chart_branch}
        git -C "$dest" checkout -B ${var.helm_chart_branch} FETCH_HEAD
      fi
    EOT
  }
}

locals {
  # Install from the local checkout when provided, else from the cloned repo under
  # this module's directory (never the caller's cwd).
  chart_path = var.local_chart_path != "" ? var.local_chart_path : "${path.module}/helm-charts/charts/amazon-cloudwatch-observability"
  # Selects every monitor except the ones resources/selector-workload.yaml labels to exclude.
  monitor_selector = {
    matchExpressions = [{ key = "e2e.cloudwatch.aws/exclude", operator = "DoesNotExist" }]
  }
}

resource "helm_release" "aws_observability" {
  name             = "amazon-cloudwatch-observability"
  chart            = local.chart_path
  namespace        = "amazon-cloudwatch"
  create_namespace = true
  timeout          = 900

  # Everything under test is configured through chart values, so the suite covers
  # the chart's own wiring (per-node default, scraperRole, TA image) as shipped.
  # Image domains are set for both `public` and the target region, because the
  # chart prefers a region-specific repositoryDomainMap entry when one exists.
  set = concat([
    { name = "clusterName", value = aws_eks_cluster.this.name },
    { name = "region", value = var.region },
    { name = "otelContainerInsights.enabled", value = tostring(var.otel_container_insights_enabled) },
    { name = "otelPrometheusMonitor.enabled", value = "true" },
    { name = "otelPrometheusMonitor.readSecrets", value = tostring(var.read_secrets) },
    # Custom operator image (per-node strategy, SM/PM CRD creation, scraper_role).
    { name = "manager.image.repositoryDomainMap.public", value = var.operator_image_domain },
    { name = "manager.image.repositoryDomainMap.${var.region}", value = var.operator_image_domain },
    { name = "manager.image.repository", value = var.operator_image_repo },
    { name = "manager.image.tag", value = var.operator_image_tag },
    # Custom Target Allocator image, used by every TA the chart renders.
    { name = "agent.prometheus.targetAllocator.image.repositoryDomainMap.public", value = var.ta_image_domain },
    { name = "agent.prometheus.targetAllocator.image.repositoryDomainMap.${var.region}", value = var.ta_image_domain },
    { name = "agent.prometheus.targetAllocator.image.repository", value = var.ta_image_repo },
    { name = "agent.prometheus.targetAllocator.image.tag", value = var.ta_image_tag },
    ], var.cwagent_image_tag == "" ? [] : [
    { name = "agent.image.repositoryDomainMap.public", value = var.cwagent_image_domain },
    { name = "agent.image.repositoryDomainMap.${var.region}", value = var.cwagent_image_domain },
    { name = "agent.image.repository", value = var.cwagent_image_repo },
    { name = "agent.image.tag", value = var.cwagent_image_tag },
  ])

  # Monitor selectors are objects, so they go in values rather than set.
  values = var.monitor_selector ? [yamlencode({
    otelPrometheusMonitor = {
      serviceMonitorSelector = local.monitor_selector
      podMonitorSelector     = local.monitor_selector
    }
  })] : []

  depends_on = [
    aws_eks_addon.pod_identity_agent,
    null_resource.kubectl,
    null_resource.clone_helm_chart,
  ]
}

# --- Pod Identity association (after Helm creates the service account) ---

resource "aws_eks_pod_identity_association" "cloudwatch_agent" {
  depends_on      = [helm_release.aws_observability]
  cluster_name    = aws_eks_cluster.this.name
  namespace       = "amazon-cloudwatch"
  service_account = "cloudwatch-agent"
  role_arn        = aws_iam_role.pod_identity_role.arn
}

# --- Restart pods to pick up Pod Identity ---

resource "null_resource" "restart_pods" {
  depends_on = [aws_eks_pod_identity_association.cloudwatch_agent]
  triggers   = { timestamp = timestamp() }
  provisioner "local-exec" {
    command = <<-EOT
      kubectl -n amazon-cloudwatch rollout restart daemonset/cloudwatch-agent
      kubectl -n amazon-cloudwatch rollout restart deployment/cloudwatch-agent-target-allocator 2>/dev/null || true
      kubectl -n amazon-cloudwatch rollout restart deployment/cloudwatch-agent-cluster-scraper 2>/dev/null || true
      kubectl -n amazon-cloudwatch rollout restart deployment/cloudwatch-agent-cluster-scraper-target-allocator 2>/dev/null || true
      kubectl -n amazon-cloudwatch rollout status daemonset/cloudwatch-agent --timeout=180s
      kubectl -n amazon-cloudwatch rollout status deployment/cloudwatch-agent-target-allocator --timeout=180s 2>/dev/null || true
      kubectl -n amazon-cloudwatch rollout status deployment/cloudwatch-agent-cluster-scraper --timeout=180s 2>/dev/null || true
      kubectl -n amazon-cloudwatch rollout status deployment/cloudwatch-agent-cluster-scraper-target-allocator --timeout=180s 2>/dev/null || true
    EOT
  }
}

# --- Apply the per-node workload (ServiceMonitor + PodMonitor + load gen). The
#     SM/PM CRs depend on the CRDs the operator creates. ---

resource "null_resource" "workload" {
  depends_on = [helm_release.aws_observability, null_resource.restart_pods]
  triggers   = { timestamp = timestamp() }
  provisioner "local-exec" {
    command = <<-EOT
      set -e
      # The operator creates the SM/PM CRDs after it starts. `kubectl wait` fails
      # at once on a CRD that does not exist yet, so first wait for both to exist,
      # then for them to be Established, rather than racing the apply.
      for i in $(seq 1 60); do
        kubectl get crd servicemonitors.monitoring.coreos.com podmonitors.monitoring.coreos.com >/dev/null 2>&1 && break
        sleep 3
      done
      kubectl wait --for=condition=Established --timeout=180s \
        crd/servicemonitors.monitoring.coreos.com \
        crd/podmonitors.monitoring.coreos.com
      kubectl apply -f ${path.module}/../../../../test/otel/pernode/resources/workload.yaml
      # Annotation-routing fixture (routed + default monitors) for the /jobs partition test.
      kubectl apply -f ${path.module}/../../../../test/otel/pernode/resources/routing-workload.yaml
      # basicAuth-from-Secret fixture for the otelPrometheusMonitor.readSecrets test.
      kubectl apply -f ${path.module}/../../../../test/otel/pernode/resources/secret-auth-workload.yaml
      # Monitor selector fixture (selected and excluded monitors).
      kubectl apply -f ${path.module}/../../../../test/otel/pernode/resources/selector-workload.yaml
      # Service identity fixture (annotated and plain workloads behind a ServiceMonitor and a PodMonitor).
      kubectl apply -f ${path.module}/../../../../test/otel/pernode/resources/identity-workload.yaml
    EOT
  }
}

# --- Test runner ---

resource "null_resource" "validator" {
  depends_on = [
    null_resource.restart_pods,
    null_resource.workload,
  ]

  triggers = { always_run = timestamp() }

  provisioner "local-exec" {
    command = <<-EOT
      echo "Running OTEL per-node integration tests"
      cd ../../../..

      # No fixed propagation sleep: the tests poll CloudWatch with a bounded retry
      # (queryWorkloadMetric) to absorb ingestion lag.
      PERNODE_OTEL_CI_ENABLED=${var.otel_container_insights_enabled} \
      PERNODE_READ_SECRETS=${var.read_secrets} \
      PERNODE_MONITOR_SELECTOR=${var.monitor_selector} \
      go test -tags integration -timeout 1h -v ${var.test_dir} \
        -eksClusterName=${aws_eks_cluster.this.name} \
        -computeType=EKS \
        -eksDeploymentStrategy=DAEMON \
        -region=${var.region}
    EOT
  }
}
