// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

data "aws_caller_identity" "current" {}

locals {
  aws_eks = "aws eks --region ${var.region}"

  efaburn_image = var.efaburn_image != "" ? var.efaburn_image : "${data.aws_caller_identity.current.account_id}.dkr.ecr.${var.region}.amazonaws.com/efaburn:latest"

  # Node 1: one efaburn pod per namespace, each with 1 of the node's 2 EFA
  # cards. otelFilters.namespaces.exclude has efa-drop, so the chart keeps
  # the efa-drop card's metrics and removes their pod, namespace and
  # container.
  efaburn_namespaces = ["efa-keep", "efa-drop"]

  # Node 2: one more efa-drop pod (efaburn-free) with 1 of the node's 2 EFA
  # cards. The other card stays free.
  efaburn_free_namespace = "efa-drop"
}

module "common" {
  source             = "../../../common"
  cwagent_image_repo = var.cwagent_image_repo
  cwagent_image_tag  = var.cwagent_image_tag
}

module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 21.0"

  name               = "cwagent-eks-integ-${module.common.testing_id}"
  kubernetes_version = var.k8s_version

  # Full role name (<=64); the module's "<name>-cluster-" name_prefix would exceed the 38-char cap.
  iam_role_use_name_prefix = false

  vpc_id     = aws_vpc.efa_test_vpc.id
  subnet_ids = aws_subnet.efa_test_public_subnet[*].id

  endpoint_public_access                   = true
  enable_cluster_creator_admin_permissions = true

  eks_managed_node_groups = {
    standard = {
      ami_type       = "AL2023_x86_64_STANDARD"
      instance_types = ["t3.medium"]
      min_size       = 1
      max_size       = 1
      desired_size   = 1
      subnet_ids     = aws_subnet.efa_test_public_subnet[*].id
    }

    multi_efa = {
      enable_efa_support = true
      ami_type           = var.ami_type
      instance_types     = [var.instance_type]
      min_size           = 1
      max_size           = 1
      desired_size       = 1
      subnet_ids         = aws_subnet.efa_test_private_subnet[*].id

      labels = {
        "ci-test.example.com/multi-efa-sm" = "true"
        "ci-test.example.com/node-color"   = "yellow"
      }
    }

    multi_efa_free = {
      enable_efa_support = true
      ami_type           = var.ami_type
      instance_types     = [var.instance_type]
      min_size           = 1
      max_size           = 1
      desired_size       = 1
      subnet_ids         = aws_subnet.efa_test_private_subnet[*].id

      labels = {
        "ci-test.example.com/multi-efa-free" = "true"
        "ci-test.example.com/node-color"     = "yellow"
      }
    }
  }

  addons = {
    coredns                = {}
    eks-pod-identity-agent = { before_compute = true }
    kube-proxy             = {}
    vpc-cni                = { before_compute = true }
  }

  tags = { Owner = "default" }
}

# --- EFA Device Plugin ---

resource "helm_release" "aws_efa_device_plugin" {
  name       = "aws-efa-k8s-device-plugin"
  repository = "https://aws.github.io/eks-charts"
  chart      = "aws-efa-k8s-device-plugin"
  version    = "v0.5.7"
  namespace  = "kube-system"
  wait       = true

  depends_on = [module.eks]
}

# --- Helm chart install ---

resource "null_resource" "kubectl" {
  depends_on = [module.eks]
  provisioner "local-exec" {
    command = "${local.aws_eks} update-kubeconfig --name ${module.eks.cluster_name}"
  }
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
  name             = "amazon-cloudwatch-observability"
  chart            = "./helm-charts/charts/amazon-cloudwatch-observability"
  namespace        = "amazon-cloudwatch"
  create_namespace = true
  wait             = false
  timeout          = 600

  set = [
    { name = "clusterName", value = module.eks.cluster_name },
    { name = "region", value = var.region },
    { name = "otelContainerInsights.enabled", value = "true" },
  ]

  values = [
    <<-EOT
      otelFilters:
        namespaces:
          exclude: ["efa-drop"]
    EOT
  ]

  # No agent runs before all three efaburn pods hold their cards. Otherwise a
  # card exports series without a pod before its pod starts, and they look
  # the same as a stripped efa-drop card.
  depends_on = [
    module.eks,
    null_resource.kubectl,
    data.external.clone_helm_chart,
    null_resource.efaburn,
    null_resource.efaburn_free,
  ]
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
# After the image patch. The agents start only after all three efaburn pods
# hold their cards (see helm_release.aws_observability), so every agent
# starts with every pod's card in use.

resource "null_resource" "restart_pods" {
  depends_on = [null_resource.update_image, null_resource.efaburn, null_resource.efaburn_free]
  triggers   = { timestamp = timestamp() }
  provisioner "local-exec" {
    command = <<-EOT
      kubectl -n amazon-cloudwatch rollout restart daemonset/cloudwatch-agent
      kubectl -n amazon-cloudwatch rollout restart deployment/cloudwatch-agent-cluster-scraper 2>/dev/null || true
      kubectl -n amazon-cloudwatch rollout status daemonset/cloudwatch-agent --timeout=120s
    EOT
  }
}

# --- efaburn workloads: one pod per namespace, each with its own EFA card ---

resource "null_resource" "efaburn" {
  for_each   = toset(local.efaburn_namespaces)
  depends_on = [module.eks, null_resource.kubectl, helm_release.aws_efa_device_plugin]
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
        name: efaburn
        namespace: ${each.key}
      spec:
        replicas: 1
        selector:
          matchLabels:
            app: efaburn
        template:
          metadata:
            labels:
              app: efaburn
          spec:
            nodeSelector:
              ci-test.example.com/multi-efa-sm: "true"
            containers:
            - name: efaburn
              image: ${local.efaburn_image}
              resources:
                limits:
                  vpc.amazonaws.com/efa: "1"
                requests:
                  vpc.amazonaws.com/efa: "1"
                  memory: 8000Mi
              securityContext:
                allowPrivilegeEscalation: false
                runAsNonRoot: true
                runAsUser: 1000
      EOF
      kubectl -n ${each.key} rollout status deployment/efaburn --timeout=10m
    EOT
  }
}

# --- efaburn-free workload: one efa-drop pod on node 2, the other card free ---
# After null_resource.efaburn, which creates the efa-drop namespace.

resource "null_resource" "efaburn_free" {
  depends_on = [module.eks, null_resource.kubectl, helm_release.aws_efa_device_plugin, null_resource.efaburn]
  provisioner "local-exec" {
    command = <<-EOT
      cat <<'EOF' | kubectl apply -f -
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: efaburn-free
        namespace: ${local.efaburn_free_namespace}
      spec:
        replicas: 1
        selector:
          matchLabels:
            app: efaburn-free
        template:
          metadata:
            labels:
              app: efaburn-free
          spec:
            nodeSelector:
              ci-test.example.com/multi-efa-free: "true"
            containers:
            - name: efaburn
              image: ${local.efaburn_image}
              resources:
                limits:
                  vpc.amazonaws.com/efa: "1"
                requests:
                  vpc.amazonaws.com/efa: "1"
                  memory: 8000Mi
              securityContext:
                allowPrivilegeEscalation: false
                runAsNonRoot: true
                runAsUser: 1000
      EOF
      kubectl -n ${local.efaburn_free_namespace} rollout status deployment/efaburn-free --timeout=10m
    EOT
  }
}

# --- Test runner ---

resource "null_resource" "validator" {
  depends_on = [
    null_resource.restart_pods,
    null_resource.efaburn,
    null_resource.efaburn_free,
  ]

  triggers = { always_run = timestamp() }

  provisioner "local-exec" {
    command = <<-EOT
      echo "Running OTEL Multi-EFA filters cluster integration tests"
      cd ../../../..

      echo "Waiting 3 minutes for metrics to propagate..."
      sleep 180

      go test -tags integration -timeout 1h -v ${var.test_dir} \
        -eksClusterName=${module.eks.cluster_name} \
        -computeType=EKS \
        -eksDeploymentStrategy=DAEMON \
        -region=${var.region}
    EOT
  }
}
