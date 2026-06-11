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
  aws_eks      = "aws eks --region ${var.region}"
  cluster_name = "cwagent-eks-integ-${module.common.testing_id}"
}

# --- EKS Cluster ---

resource "aws_eks_cluster" "this" {
  name     = local.cluster_name
  role_arn = module.basic_components.role_arn
  version  = var.k8s_version
  vpc_config {
    subnet_ids         = module.basic_components.public_subnet_ids
    security_group_ids = [module.basic_components.security_group]
  }
}

# --- Node IAM Role ---

resource "aws_iam_role" "node_role" {
  name = "cwagent-otel-efa-Worker-Role-${module.common.testing_id}"
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

# --- Standard node group (for operator/CoreDNS) ---

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
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy,
    aws_iam_role_policy_attachment.node_AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.node_AmazonEC2ContainerRegistryReadOnly,
  ]
}

# --- EFA launch template ---
# EFA requires network_interfaces with interface_type = "efa" in the launch
# template. The node group's subnet_ids must be a single subnet so the
# launch template's EFA interface is placed in a valid AZ (EFA-capable
# instance types are constrained to specific AZs).

data "aws_subnet" "first" {
  id = module.basic_components.public_subnet_ids[0]
}

resource "aws_launch_template" "efa" {
  name_prefix = "otel-efa-${module.common.testing_id}-"

  network_interfaces {
    device_index   = 0
    interface_type = "efa"
  }
}

# --- EFA workload node group ---

resource "aws_eks_node_group" "efa_workload" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "efa-workload-${module.common.testing_id}"
  node_role_arn   = aws_iam_role.node_role.arn

  # subnet_ids is required by the API even with a launch template.
  # Use the same single subnet as the launch template.
  subnet_ids = [data.aws_subnet.first.id]

  scaling_config {
    desired_size = 1
    max_size     = 1
    min_size     = 1
  }

  ami_type       = var.ami_type
  capacity_type  = "ON_DEMAND"
  instance_types = [var.instance_type]

  launch_template {
    id      = aws_launch_template.efa.id
    version = aws_launch_template.efa.latest_version
  }

  labels = {
    "ci-test.example.com/node-color" = "yellow"
  }

  taint {
    key    = "vpc.amazonaws.com/efa"
    value  = "true"
    effect = "NO_SCHEDULE"
  }

  depends_on = [
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy,
    aws_iam_role_policy_attachment.node_AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.node_AmazonEC2ContainerRegistryReadOnly,
  ]
}

# --- EFA idle node group ---

resource "aws_eks_node_group" "efa_idle" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "efa-idle-${module.common.testing_id}"
  node_role_arn   = aws_iam_role.node_role.arn
  subnet_ids      = [data.aws_subnet.first.id]

  scaling_config {
    desired_size = 1
    max_size     = 1
    min_size     = 1
  }

  ami_type       = var.ami_type
  capacity_type  = "ON_DEMAND"
  instance_types = [var.instance_type]

  launch_template {
    id      = aws_launch_template.efa.id
    version = aws_launch_template.efa.latest_version
  }

  labels = {
    "ci-test.example.com/node-color" = "yellow"
  }

  taint {
    key    = "vpc.amazonaws.com/efa"
    value  = "true"
    effect = "NO_SCHEDULE"
  }

  depends_on = [
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy,
    aws_iam_role_policy_attachment.node_AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.node_AmazonEC2ContainerRegistryReadOnly,
  ]
}

# --- Pod Identity IAM Role ---

resource "aws_iam_role" "pod_identity_role" {
  name = "cwagent-otel-efa-pod-identity-${module.common.testing_id}"
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
  depends_on   = [aws_eks_node_group.standard]
  cluster_name = aws_eks_cluster.this.name
  addon_name   = "eks-pod-identity-agent"
}

# --- Update kubeconfig ---

resource "null_resource" "kubectl" {
  depends_on = [aws_eks_cluster.this, aws_eks_node_group.standard]
  provisioner "local-exec" {
    command = "${local.aws_eks} update-kubeconfig --name ${aws_eks_cluster.this.name}"
  }
}

# --- EFA device plugin via Helm ---

resource "helm_release" "aws_efa_device_plugin" {
  depends_on = [aws_eks_node_group.efa_workload, aws_eks_node_group.efa_idle, null_resource.kubectl]

  name       = "aws-efa-k8s-device-plugin"
  repository = "https://aws.github.io/eks-charts"
  chart      = "aws-efa-k8s-device-plugin"
  version    = "v0.5.7"
  namespace  = "kube-system"
  wait       = true

  values = [
    <<-EOT
      tolerations:
        - key: vpc.amazonaws.com/efa
          operator: Exists
          effect: NoSchedule
    EOT
  ]
}

# --- Helm chart install (observability) ---

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
      kubectl -n amazon-cloudwatch rollout status deployment/cloudwatch-agent-cluster-scraper --timeout=120s 2>/dev/null || true
    EOT
  }
}

# --- EFA workloads ---

resource "null_resource" "efa_workloads" {
  depends_on = [
    helm_release.aws_efa_device_plugin,
    aws_eks_node_group.efa_workload,
    aws_eks_node_group.efa_idle,
    null_resource.kubectl,
  ]

  provisioner "local-exec" {
    command = <<-EOT
      # Wait for EFA device plugin pods to be ready
      kubectl -n kube-system rollout status daemonset/aws-efa-k8s-device-plugin --timeout=300s

      # Deploy efaburn Deployment
      cat <<'EOF' | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: efaburn
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
      app: efaburn
  template:
    metadata:
      labels:
        app: efaburn
        ci-test.example.com/pod-color: teal
    spec:
      tolerations:
        - key: vpc.amazonaws.com/efa
          operator: Exists
          effect: NoSchedule
      nodeSelector:
        node.kubernetes.io/instance-type: ${var.instance_type}
      affinity:
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            - labelSelector:
                matchLabels:
                  app: efa-test-allocated-marker
              topologyKey: kubernetes.io/hostname
      containers:
        - name: efaburn
          image: ${var.efaburn_image}
          resources:
            limits:
              memory: 8000Mi
              vpc.amazonaws.com/efa: "1"
            requests:
              memory: 8000Mi
              vpc.amazonaws.com/efa: "1"
          securityContext:
            allowPrivilegeEscalation: false
            runAsNonRoot: true
            runAsUser: 1000
EOF

      # Deploy efa-test-allocated bare Pod
      cat <<'EOF' | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: efa-test-allocated
  namespace: default
  labels:
    app: efa-test-allocated-marker
spec:
  tolerations:
    - key: vpc.amazonaws.com/efa
      operator: Exists
      effect: NoSchedule
  nodeSelector:
    node.kubernetes.io/instance-type: ${var.instance_type}
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        - labelSelector:
            matchLabels:
              app: efaburn
          topologyKey: kubernetes.io/hostname
  containers:
    - name: efa-test
      image: amazonlinux:2
      command: ["sleep", "infinity"]
      resources:
        limits:
          vpc.amazonaws.com/efa: "1"
        requests:
          vpc.amazonaws.com/efa: "1"
EOF

      # Wait for workloads
      kubectl -n default rollout status deployment/efaburn --timeout=300s
      kubectl -n default wait --for=condition=Ready pod/efa-test-allocated --timeout=300s
    EOT
  }
}

# --- Test runner ---

resource "null_resource" "validator" {
  depends_on = [
    null_resource.restart_pods,
    null_resource.efa_workloads,
  ]

  triggers = { always_run = timestamp() }

  provisioner "local-exec" {
    command = <<-EOT
      echo "Running OTEL EFA cluster integration tests"
      cd ../../../..

      echo "Waiting 6 minutes for metrics to propagate (covers Zeus 5-min staleness window)..."
      sleep 360

      go test -tags integration -timeout 1h -v ${var.test_dir} \
        -eksClusterName=${aws_eks_cluster.this.name} \
        -computeType=EKS \
        -eksDeploymentStrategy=DAEMON \
        -region=${var.region}

    EOT
  }
}
