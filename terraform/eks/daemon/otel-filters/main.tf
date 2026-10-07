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

  # Node labels kept or removed by the nodeLabels filter.
  node_labels = {
    "filters-e2e-node-keep"          = "v"
    "filters-e2e-node-drop"          = "v"
    "filters-e2e.example.com/drop-a" = "v"
    "filters-e2e.example.com/drop-b" = "v"
  }

  # One workload per namespace. The namespaces filter keeps the first two.
  namespaces = ["filters-keep-a", "filters-exact", "filters-keep-secret-b", "filters-other"]
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

resource "aws_eks_node_group" "this" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "cwagent-filters-${module.common.testing_id}"
  node_role_arn   = aws_iam_role.node_role.arn
  subnet_ids      = module.basic_components.public_subnet_ids

  scaling_config {
    desired_size = 2
    max_size     = 2
    min_size     = 2
  }

  ami_type       = var.ami_type
  capacity_type  = "ON_DEMAND"
  disk_size      = 20
  instance_types = [var.instance_type]
  labels         = local.node_labels

  depends_on = [
    aws_iam_role_policy_attachment.node_AmazonEC2ContainerRegistryReadOnly,
    aws_iam_role_policy_attachment.node_AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy,
  ]
}

# EKS Node IAM Role
resource "aws_iam_role" "node_role" {
  name = "cwagent-filters-eks-Worker-Role-${module.common.testing_id}"
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
  name = "cwagent-filters-pod-identity-${module.common.testing_id}"
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

  set = [
    { name = "clusterName", value = aws_eks_cluster.this.name },
    { name = "region", value = var.region },
    { name = "otelContainerInsights.enabled", value = "true" },
  ]

  # Each filter has an include and an exclude, with at least one prefix.
  # Label include lists must be the same for metrics and logs.
  # pod-template-hash is included but is also a recommended exclusion, which
  # defaults to on for metrics and off for logs.
  values = [
    <<-EOT
      otelContainerInsights:
        filters:
          metrics:
            namespaces:
              include: ["filters-keep-*", "filters-exact"]
              exclude: ["filters-keep-secret*"]
            nodeLabels:
              include: ["filters-e2e*", "kubernetes.io/os"]
              exclude: ["filters-e2e-node-drop", "filters-e2e.example.com/*"]
            podLabels:
              include: ["app", "filters-e2e-keep*", "pod-template-hash"]
              exclude: ["filters-e2e-keep-secret"]
            metricNames:
              include: ["container_*", "kube_pod_info", "k8s.pod.*"]
              exclude: ["container_network_*", "container_memory_usage_bytes"]
          logs:
            namespaces:
              include: ["filters-keep-*", "filters-exact"]
              exclude: ["filters-keep-secret*"]
            nodeLabels:
              include: ["filters-e2e*", "kubernetes.io/os"]
              exclude: ["filters-e2e-node-drop", "filters-e2e.example.com/*"]
            podLabels:
              include: ["app", "filters-e2e-keep*", "pod-template-hash"]
              exclude: ["filters-e2e-keep-secret"]
    EOT
  ]

  depends_on = [
    aws_eks_addon.pod_identity_agent,
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

# --- Test workloads: one logging deployment per namespace ---

resource "null_resource" "workloads" {
  for_each   = toset(local.namespaces)
  depends_on = [aws_eks_node_group.this, null_resource.kubectl]
  provisioner "local-exec" {
    command = <<-EOT
      cat <<'EOF' | kubectl apply -f -
      apiVersion: v1
      kind: Namespace
      metadata:
        name: ${each.key}
      ---
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: filters-e2e
        namespace: ${each.key}
      spec:
        replicas: 1
        selector:
          matchLabels:
            app: filters-e2e
        template:
          metadata:
            labels:
              app: filters-e2e
              filters-e2e-keep-a: v
              filters-e2e-keep-secret: v
              filters-e2e-other: v
          spec:
            containers:
            - name: logger
              image: busybox:1.36
              command: ["sh", "-c", "while true; do echo otel-filters-e2e ${each.key}; sleep 5; done"]
              resources:
                requests:
                  cpu: 10m
                  memory: 16Mi
      EOF
      kubectl -n ${each.key} rollout status deployment/filters-e2e --timeout=5m
    EOT
  }
}

# --- Test runner ---

resource "null_resource" "validator" {
  depends_on = [
    null_resource.restart_pods,
    null_resource.workloads,
  ]

  triggers = { always_run = timestamp() }

  provisioner "local-exec" {
    command = <<-EOT
      echo "Running OTEL filters cluster integration tests"
      cd ../../../..

      echo "Waiting 3 minutes for metrics and logs to propagate..."
      sleep 180

      go test -tags integration -timeout 1h -v ${var.test_dir} \
        -eksClusterName=${aws_eks_cluster.this.name} \
        -computeType=EKS \
        -eksDeploymentStrategy=DAEMON \
        -region=${var.region}
    EOT
  }
}
