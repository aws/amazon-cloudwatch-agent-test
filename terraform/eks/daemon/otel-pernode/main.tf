# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
# SPDX-License-Identifier: MIT
#
# E2E harness for the Target Allocator per-node allocation strategy plus zero-step
# ServiceMonitor/PodMonitor CRD bundling (G1) and TA resilience to missing CRDs
# (G2). Compared with ../otel (the standard suite) this harness:
#   * installs a helm-charts checkout that BUNDLES the SM/PM CRDs (no separate
#     prometheus-operator CRD install -- that is the whole point of G1),
#   * deploys CUSTOM operator and Target Allocator images carrying the per-node +
#     CRD-watch code (the public images do not have it),
#   * forces the per-node allocation strategy on the AmazonCloudWatchAgent CR,
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

# --- Clone the helm-charts checkout that BUNDLES the SM/PM CRDs (G1). ---
# NOTE: no prometheus-operator CRD install step exists in this harness on
# purpose -- the SM/PM CRDs must come from the chart alone.

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
}

resource "helm_release" "aws_observability" {
  name             = "amazon-cloudwatch-observability"
  chart            = local.chart_path
  namespace        = "amazon-cloudwatch"
  create_namespace = true
  timeout          = 900

  set = [
    { name = "clusterName", value = aws_eks_cluster.this.name },
    { name = "region", value = var.region },
    { name = "otelContainerInsights.enabled", value = "true" },
    # Custom operator image (carries the per-node strategy + CRD-watch resilience).
    { name = "manager.image.repositoryDomainMap.public", value = var.operator_image_domain },
    { name = "manager.image.repository", value = var.operator_image_repo },
    { name = "manager.image.tag", value = var.operator_image_tag },
  ]

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

# --- Patch the CR: custom agent image + custom Target Allocator image + force
#     the per-node allocation strategy. The TA image is not a first-class chart
#     value, so it is patched onto the AmazonCloudWatchAgent CR directly
#     (mirrors scripts/deploy-all.sh CUSTOM_TA). ---

resource "null_resource" "patch_cr" {
  depends_on = [helm_release.aws_observability, null_resource.kubectl]
  triggers   = { timestamp = timestamp() }
  provisioner "local-exec" {
    command = <<-EOT
      set -e
      # Wait for readiness instead of a fixed sleep: the operator must register the
      # AmazonCloudWatchAgent CRD and create the CR before we can patch it.
      kubectl wait --for=condition=Established --timeout=180s \
        crd/amazoncloudwatchagents.cloudwatch.aws.amazon.com
      for i in $(seq 1 60); do
        kubectl -n amazon-cloudwatch get AmazonCloudWatchAgent cloudwatch-agent >/dev/null 2>&1 && break
        [ "$i" = "60" ] && { echo "CR cloudwatch-agent never appeared" >&2; exit 1; }
        sleep 5
      done
      kubectl -n amazon-cloudwatch patch AmazonCloudWatchAgent cloudwatch-agent --type='json' \
        -p='[{"op": "replace", "path": "/spec/image", "value": "${var.cwagent_image_repo}:${var.cwagent_image_tag}"}]'
      kubectl -n amazon-cloudwatch patch AmazonCloudWatchAgent cloudwatch-agent --type=merge \
        -p='{"spec":{"targetAllocator":{"allocationStrategy":"${var.allocation_strategy}","image":"${var.ta_image}"}}}'
    EOT
  }
}

# --- Restart pods to pick up Pod Identity + new images ---

resource "null_resource" "restart_pods" {
  depends_on = [aws_eks_pod_identity_association.cloudwatch_agent, null_resource.patch_cr]
  triggers   = { timestamp = timestamp() }
  provisioner "local-exec" {
    command = <<-EOT
      kubectl -n amazon-cloudwatch rollout restart daemonset/cloudwatch-agent
      kubectl -n amazon-cloudwatch rollout restart deployment/cloudwatch-agent-target-allocator 2>/dev/null || true
      kubectl -n amazon-cloudwatch rollout status daemonset/cloudwatch-agent --timeout=180s
      kubectl -n amazon-cloudwatch rollout status deployment/cloudwatch-agent-target-allocator --timeout=180s 2>/dev/null || true
    EOT
  }
}

# --- Apply the per-node workload (ServiceMonitor + PodMonitor + load gen). The
#     SM/PM CRs depend on the CRDs the chart bundled. ---

resource "null_resource" "workload" {
  depends_on = [helm_release.aws_observability, null_resource.restart_pods]
  triggers   = { timestamp = timestamp() }
  provisioner "local-exec" {
    command = <<-EOT
      set -e
      # The SM/PM CRs require the chart's bundled CRDs to be Established first;
      # wait rather than racing CRD creation with the apply.
      kubectl wait --for=condition=Established --timeout=180s \
        crd/servicemonitors.monitoring.coreos.com \
        crd/podmonitors.monitoring.coreos.com
      kubectl apply -f ${path.module}/../../../../test/otel/pernode/resources/workload.yaml
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
      go test -tags integration -timeout 1h -v ${var.test_dir} \
        -eksClusterName=${aws_eks_cluster.this.name} \
        -computeType=EKS \
        -eksDeploymentStrategy=DAEMON \
        -region=${var.region}
    EOT
  }
}
