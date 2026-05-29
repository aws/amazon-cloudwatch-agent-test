# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
# SPDX-License-Identifier: MIT

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

# EKS Node IAM Role
resource "aws_iam_role" "node_role" {
  name = "cwagent-otel-gpu-Worker-Role-${module.common.testing_id}"
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

# --- Standard Node Group (for operator, CoreDNS, KSM) ---

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

# --- GPU Node Groups ---

# gpu-single: 1x g4dn.xlarge (idle — no workload scheduled)
resource "aws_eks_node_group" "gpu_single" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "gpu-single-${module.common.testing_id}"
  node_role_arn   = aws_iam_role.node_role.arn
  subnet_ids      = module.basic_components.public_subnet_ids

  scaling_config {
    desired_size = 1
    max_size     = 1
    min_size     = 1
  }

  ami_type       = var.ami_type
  capacity_type  = "ON_DEMAND"
  disk_size      = 20
  instance_types = [var.instance_type]

  labels = {
    "nvidia.com/gpu.present"         = "true"
    "ci-test.example.com/node-color" = "green"
  }

  taint {
    key    = "nvidia.com/gpu"
    value  = "true"
    effect = "NO_SCHEDULE"
  }

  depends_on = [
    aws_iam_role_policy_attachment.node_AmazonEC2ContainerRegistryReadOnly,
    aws_iam_role_policy_attachment.node_AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy,
  ]
}

# gpu-multi: 1x g4dn.12xlarge (multi-gpu-burn workload)
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

  ami_type       = "AL2023_x86_64_NVIDIA"
  capacity_type  = "ON_DEMAND"
  disk_size      = 20
  instance_types = ["g4dn.12xlarge"]

  labels = {
    "nvidia.com/gpu.present"         = "true"
    "ci-test.example.com/node-color" = "green"
    "ci-test.example.com/multi-gpu"  = "true"
  }

  taint {
    key    = "nvidia.com/gpu"
    value  = "true"
    effect = "NO_SCHEDULE"
  }

  depends_on = [
    aws_iam_role_policy_attachment.node_AmazonEC2ContainerRegistryReadOnly,
    aws_iam_role_policy_attachment.node_AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy,
  ]
}

# --- NVIDIA Device Plugin (Helm) ---

resource "helm_release" "nvidia_device_plugin" {
  name       = "nvidia-device-plugin"
  repository = "https://nvidia.github.io/k8s-device-plugin"
  chart      = "nvidia-device-plugin"
  namespace  = "kube-system"

  set = [
    { name = "tolerations[0].key", value = "nvidia.com/gpu" },
    { name = "tolerations[0].operator", value = "Exists" },
    { name = "tolerations[0].effect", value = "NoSchedule" }
  ]

  depends_on = [
    aws_eks_node_group.gpu_single,
    aws_eks_node_group.gpu_multi,
    null_resource.kubectl,
  ]
}

# --- NVIDIA Toolkit-Ready DaemonSet ---

resource "null_resource" "nvidia_toolkit_ready" {
  depends_on = [aws_eks_node_group.gpu_single, aws_eks_node_group.gpu_multi, null_resource.kubectl]
  provisioner "local-exec" {
    command = <<-EOT
      cat <<'EOF' | kubectl apply -f -
      apiVersion: apps/v1
      kind: DaemonSet
      metadata:
        name: nvidia-toolkit-ready
        namespace: kube-system
      spec:
        selector:
          matchLabels:
            app: nvidia-toolkit-ready
        template:
          metadata:
            labels:
              app: nvidia-toolkit-ready
          spec:
            tolerations:
            - key: nvidia.com/gpu
              operator: Exists
              effect: NoSchedule
            affinity:
              nodeAffinity:
                requiredDuringSchedulingIgnoredDuringExecution:
                  nodeSelectorTerms:
                  - matchExpressions:
                    - key: nvidia.com/gpu.present
                      operator: Exists
            initContainers:
            - name: create-marker
              image: busybox:stable
              command: ["sh", "-c", "mkdir -p /run/nvidia/validations && touch /run/nvidia/validations/toolkit-ready"]
              volumeMounts:
              - name: run-nvidia
                mountPath: /run/nvidia
                mountPropagation: Bidirectional
              securityContext:
                privileged: true
            containers:
            - name: pause
              image: registry.k8s.io/pause:3.9
            volumes:
            - name: run-nvidia
              hostPath:
                path: /run/nvidia
      EOF
    EOT
  }
}

# --- multi-gpu-burn Deployment (g4dn.12xlarge only) ---

resource "null_resource" "multi_gpu_burn" {
  depends_on = [helm_release.nvidia_device_plugin, null_resource.nvidia_toolkit_ready]
  provisioner "local-exec" {
    command = <<-EOT
      cat <<'EOF' | kubectl apply -f -
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: multi-gpu-burn
        namespace: default
      spec:
        replicas: 1
        revisionHistoryLimit: 2
        progressDeadlineSeconds: 300
        strategy:
          type: RollingUpdate
          rollingUpdate:
            maxSurge: 0
            maxUnavailable: 1
        selector:
          matchLabels:
            app: multi-gpu-burn
        template:
          metadata:
            labels:
              app: multi-gpu-burn
              ci-test.example.com/pod-color: magenta
          spec:
            tolerations:
            - key: nvidia.com/gpu
              operator: Exists
              effect: NoSchedule
            nodeSelector:
              node.kubernetes.io/instance-type: g4dn.12xlarge
            containers:
            - name: gpu-burn
              image: chrstnhntschl/gpu_burn:latest
              args: ["3600"]
              resources:
                limits:
                  nvidia.com/gpu: "1"
      EOF
    EOT
  }
}

# --- Pod Identity IAM Role ---

resource "aws_iam_role" "pod_identity_role" {
  name = "cwagent-otel-gpu-pod-identity-${module.common.testing_id}"
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
  depends_on   = [aws_eks_node_group.gpu_single, aws_eks_node_group.gpu_multi]
  cluster_name = aws_eks_cluster.this.name
  addon_name   = "eks-pod-identity-agent"
}

# --- Update kubeconfig ---

resource "null_resource" "kubectl" {
  depends_on = [aws_eks_cluster.this, aws_eks_node_group.gpu_single, aws_eks_node_group.gpu_multi]
  provisioner "local-exec" {
    command = "${local.aws_eks} update-kubeconfig --name ${aws_eks_cluster.this.name}"
  }
}

# --- Helm chart install ---

data "external" "clone_helm_chart" {
  program = ["bash", "-c", <<-EOT
    rm -rf ./helm-charts
    git clone -b ${var.helm_chart_branch} https://github.com/aws-observability/helm-charts.git ./helm-charts
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
    helm_release.nvidia_device_plugin,
    null_resource.nvidia_toolkit_ready,
    null_resource.kubectl,
    data.external.clone_helm_chart,
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

# --- Patch agent image (both DaemonSet and cluster-scraper) ---

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

# --- Restart pods to pick up Pod Identity + new image ---

resource "null_resource" "restart_pods" {
  depends_on = [aws_eks_pod_identity_association.cloudwatch_agent, null_resource.update_image]
  triggers   = { timestamp = timestamp() }
  provisioner "local-exec" {
    command = <<-EOT
      kubectl -n amazon-cloudwatch rollout restart daemonset/cloudwatch-agent
      kubectl -n amazon-cloudwatch rollout restart deployment/cloudwatch-agent-cluster-scraper 2>/dev/null || true
      kubectl -n amazon-cloudwatch rollout status daemonset/cloudwatch-agent --timeout=120s
      kubectl -n amazon-cloudwatch rollout status deployment/cloudwatch-agent-cluster-scraper --timeout=120s 2>/dev/null || true
    EOT
  }
}

# --- Test runner ---

resource "null_resource" "validator" {
  depends_on = [
    null_resource.restart_pods,
    null_resource.multi_gpu_burn,
  ]

  triggers = { always_run = timestamp() }

  provisioner "local-exec" {
    command = <<-EOT
      echo "Running OTEL GPU cluster integration tests"
      cd ../../../..

      echo "Waiting for DCGM exporter pods to be ready..."
      for i in $(seq 1 30); do
        READY=$(kubectl get pods -n amazon-cloudwatch -l app.kubernetes.io/name=dcgm-exporter -o jsonpath='{.items[*].status.phase}' 2>/dev/null | tr ' ' '\n' | grep -c Running || true)
        if [ "$READY" -ge 1 ] 2>/dev/null; then break; fi
        sleep 20
      done

      echo "Waiting 6 minutes for GPU metrics to propagate (covers Zeus 5-min staleness window)..."
      sleep 360

      go test -tags integration -timeout 1h -v ${var.test_dir} \
        -eksClusterName=${aws_eks_cluster.this.name} \
        -computeType=EKS \
        -eksDeploymentStrategy=DAEMON \
        -region=${var.region}
    EOT
  }
}
