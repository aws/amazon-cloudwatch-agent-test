# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
# SPDX-License-Identifier: MIT

# GPU DRA-path integration test cluster. Mirrors otel-gpu but exposes GPUs via the
# NVIDIA DRA driver (DeviceClass gpu.nvidia.com) instead of the device plugin, and
# the burn workload claims a GPU via a ResourceClaimTemplate. One multi-GPU node
# (g4dn.12xlarge = 4 T4) proves per-device DRA correlation (1 claimed GPU -> the
# burn pod, the other 3 uncorrelated).

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
  name     = "cwagent-eks-integ-${module.common.testing_id}"
  role_arn = module.basic_components.role_arn
  version  = var.k8s_version
  vpc_config {
    subnet_ids         = module.basic_components.public_subnet_ids
    security_group_ids = [module.basic_components.security_group]
  }
}

# --- IAM ---

resource "aws_iam_role" "node_role" {
  name = "cwagent-otel-gpu-dra-Worker-Role-${module.common.testing_id}"
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

resource "aws_iam_role" "pod_identity_role" {
  name = "cwagent-otel-gpu-dra-pod-identity-${module.common.testing_id}"
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

# --- Node Groups ---

resource "aws_eks_node_group" "standard" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "standard-${module.common.testing_id}"
  node_role_arn   = aws_iam_role.node_role.arn
  subnet_ids      = module.basic_components.public_subnet_ids

  scaling_config {
    desired_size = 1
    max_size     = 1
    min_size     = 1
  }

  ami_type       = "AL2023_x86_64_STANDARD"
  capacity_type  = "ON_DEMAND"
  disk_size      = 20
  instance_types = ["t3.medium"]

  depends_on = [
    aws_iam_role_policy_attachment.node_AmazonEC2ContainerRegistryReadOnly,
    aws_iam_role_policy_attachment.node_AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy,
  ]
}

# Multi-GPU node (g4dn.12xlarge, 4 GPUs). No taint: dedicated by being the only GPU
# node; the DRA driver DaemonSet + burn pod land here via scheduling/nodeSelector.
resource "aws_eks_node_group" "gpu_multi" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "gpu-multi-${module.common.testing_id}"
  node_role_arn   = aws_iam_role.node_role.arn
  subnet_ids      = module.basic_components.public_subnet_ids

  scaling_config {
    desired_size = 1
    max_size     = 1
    min_size     = 1
  }

  ami_type       = var.ami_type
  capacity_type  = "ON_DEMAND"
  disk_size      = 40
  instance_types = [var.instance_type]

  labels = {
    "nvidia.com/gpu.present"         = "true"
    "ci-test.example.com/node-color" = "green"
  }

  depends_on = [
    aws_iam_role_policy_attachment.node_AmazonEC2ContainerRegistryReadOnly,
    aws_iam_role_policy_attachment.node_AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy,
  ]
}

# --- EKS Addon: Pod Identity agent ---

resource "aws_eks_addon" "pod_identity_agent" {
  depends_on   = [aws_eks_node_group.standard]
  cluster_name = aws_eks_cluster.this.name
  addon_name   = "eks-pod-identity-agent"
}

# --- Update kubeconfig ---

resource "null_resource" "kubectl" {
  depends_on = [aws_eks_cluster.this, aws_eks_node_group.standard, aws_eks_node_group.gpu_multi]
  provisioner "local-exec" {
    command = "${local.aws_eks} update-kubeconfig --name ${aws_eks_cluster.this.name}"
  }
}

# --- NVIDIA DRA driver (Helm) — replaces the device plugin ---
#
# NVIDIA DRA driver install, tuned for a single-node whole-GPU EKS cluster. Verified
# against chart 25.12.0:
#   - resources.gpus.enabled=true turns on whole-GPU allocation (DeviceClass
#     gpu.nvidia.com), but the chart hard-guards it behind gpuResourcesEnabledOverride
#     =true (it refuses to co-exist with the standard GPU device plugin until KEP 5004
#     is GA). We do not run the device plugin, so the override is safe and required —
#     without it helm template fails a validation.yaml assertion.
#   - resources.computeDomains.enabled=false drops the ComputeDomain controller, whose
#     nodeAffinity requires node-role.kubernetes.io/control-plane. EKS has no such
#     (customer-visible) nodes, so the controller would stay Pending and, with helm's
#     default wait=true, time out the apply. We only need single-node whole-GPU
#     correlation, so ComputeDomains (multi-node GPU/IMEX) is unnecessary.
#   - kubeletPlugin.nodeSelector pins the plugin DaemonSet to the GPU node. The chart's
#     default kubeletPlugin nodeAffinity already ORs in nvidia.com/gpu.present=true
#     (which our nodegroup sets), so no affinity clearing is needed on this chart.
# After apply, `kubectl get deviceclass gpu.nvidia.com` and a ResourceSlice for the
# node should exist.
resource "helm_release" "nvidia_dra_driver" {
  depends_on = [aws_eks_node_group.gpu_multi, null_resource.kubectl]

  name             = "nvidia-dra-driver-gpu"
  repository       = var.nvidia_dra_repo
  chart            = "nvidia-dra-driver-gpu"
  namespace        = "nvidia-dra-driver-gpu"
  create_namespace = true
  version          = var.nvidia_dra_chart_version != "" ? var.nvidia_dra_chart_version : null

  set = [
    { name = "gpuResourcesEnabledOverride", value = "true" },
    { name = "resources.gpus.enabled", value = "true" },
    { name = "resources.computeDomains.enabled", value = "false" },
    # nodeSelector map values must be strings; type=string stops the provider
    # coercing "true" to a bool (which fails DaemonSet unmarshalling).
    { name = "kubeletPlugin.nodeSelector.nvidia\\.com/gpu\\.present", value = "true", type = "string" },
  ]
}

# --- multi-gpu-burn-dra Deployment: claims 1 GPU via DRA ---

resource "null_resource" "gpu_burn_dra" {
  depends_on = [helm_release.nvidia_dra_driver, null_resource.kubectl]
  provisioner "local-exec" {
    command = <<-EOT
      cat <<'EOF' | kubectl apply -f -
      apiVersion: resource.k8s.io/v1
      kind: ResourceClaimTemplate
      metadata:
        name: gpu-1
        namespace: default
      spec:
        spec:
          devices:
            requests:
            - name: gpu
              exactly:
                deviceClassName: gpu.nvidia.com
                count: 1
      ---
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: gpu-burn-dra
        namespace: default
      spec:
        replicas: 1
        revisionHistoryLimit: 2
        progressDeadlineSeconds: 300
        selector:
          matchLabels:
            app: gpu-burn-dra
        template:
          metadata:
            labels:
              app: gpu-burn-dra
              ci-test.example.com/pod-color: magenta
          spec:
            nodeSelector:
              node.kubernetes.io/instance-type: ${var.instance_type}
            resourceClaims:
            - name: gpu
              resourceClaimTemplateName: gpu-1
            containers:
            - name: gpu-burn
              image: chrstnhntschl/gpu_burn:latest
              args: ["3600"]
              resources:
                claims:
                - name: gpu
      EOF
    EOT
  }
}

# --- Helm chart install (observability) ---

data "external" "clone_helm_chart" {
  program = ["bash", "-c", <<-EOT
    rm -rf ./helm-charts
    git clone -b ${var.helm_chart_branch} ${var.helm_chart_repo_url} ./helm-charts
    echo '{"status":"ready"}'
  EOT
  ]
}

resource "helm_release" "aws_observability" {
  name             = "amazon-cloudwatch-observability"
  chart            = "./helm-charts/charts/amazon-cloudwatch-observability"
  namespace        = "amazon-cloudwatch"
  create_namespace = true
  wait             = false
  timeout          = 600

  set = [
    { name = "clusterName", value = aws_eks_cluster.this.name },
    { name = "region", value = var.region },
    { name = "otelContainerInsights.enabled", value = "true" },
  ]

  depends_on = [
    aws_eks_addon.pod_identity_agent,
    null_resource.kubectl,
    data.external.clone_helm_chart,
  ]
}

# --- Pod Identity association ---

resource "aws_eks_pod_identity_association" "cloudwatch_agent" {
  depends_on      = [helm_release.aws_observability]
  cluster_name    = aws_eks_cluster.this.name
  namespace       = "amazon-cloudwatch"
  service_account = "cloudwatch-agent"
  role_arn        = aws_iam_role.pod_identity_role.arn
}

# --- Patch agent image ---

resource "null_resource" "update_image" {
  depends_on = [helm_release.aws_observability, null_resource.kubectl]
  triggers   = { timestamp = timestamp() }
  provisioner "local-exec" {
    command = <<-EOT
      sleep 30
      kubectl -n amazon-cloudwatch patch AmazonCloudWatchAgent cloudwatch-agent --type='json' \
        -p='[{"op": "replace", "path": "/spec/image", "value": "${var.cwagent_image_repo}:${var.cwagent_image_tag}"}]'
      kubectl -n amazon-cloudwatch patch AmazonCloudWatchAgent cloudwatch-agent-cluster-scraper --type='json' \
        -p='[{"op": "replace", "path": "/spec/image", "value": "${var.cwagent_image_repo}:${var.cwagent_image_tag}"}]' 2>/dev/null || true
      sleep 10
    EOT
  }
}

# --- Restart pods ---

resource "null_resource" "restart_pods" {
  depends_on = [aws_eks_pod_identity_association.cloudwatch_agent, null_resource.update_image]
  triggers   = { timestamp = timestamp() }
  provisioner "local-exec" {
    command = <<-EOT
      kubectl -n amazon-cloudwatch rollout restart daemonset/cloudwatch-agent
      kubectl -n amazon-cloudwatch rollout restart deployment/cloudwatch-agent-cluster-scraper 2>/dev/null || true
      kubectl -n amazon-cloudwatch rollout status daemonset/cloudwatch-agent --timeout=120s
    EOT
  }
}

# --- Test runner ---

resource "null_resource" "validator" {
  depends_on = [null_resource.restart_pods, null_resource.gpu_burn_dra]
  triggers   = { always_run = timestamp() }
  provisioner "local-exec" {
    command = <<-EOT
      echo "Running OTEL GPU DRA cluster integration tests"
      cd ../../../..

      echo "Waiting for dcgm-exporter pods to be ready..."
      for i in $(seq 1 30); do
        READY=$(kubectl get pods -n amazon-cloudwatch -l app.kubernetes.io/name=dcgm-exporter -o jsonpath='{.items[*].status.phase}' 2>/dev/null | tr ' ' '\n' | grep -c Running || true)
        if [ "$READY" -ge 1 ] 2>/dev/null; then break; fi
        sleep 20
      done

      echo "Waiting 6 minutes for GPU metrics to propagate (Zeus 5-min staleness window)..."
      sleep 360

      go test -tags integration -timeout 1h -v ${var.test_dir} \
        -eksClusterName=${aws_eks_cluster.this.name} \
        -computeType=EKS \
        -eksDeploymentStrategy=DAEMON \
        -region=${var.region}
    EOT
  }
}
