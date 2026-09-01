// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

locals {
  aws_eks = "aws eks --region ${var.region}"
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
      # enable_efa_support wires EFA network interfaces into the launch template.
      # It does NOT install the EFA k8s device plugin (that is a separate helm
      # release on the device-plugin path); here EFA is exposed via DRA instead.
      enable_efa_support = true
      ami_type           = var.ami_type
      instance_types     = [var.instance_type]
      min_size           = 1
      max_size           = 1
      desired_size       = 1
      subnet_ids         = aws_subnet.efa_test_private_subnet[*].id

      labels = {
        "ci-test.example.com/multi-efa-dra" = "true"
        "ci-test.example.com/node-color"    = "yellow"
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

# --- DRA driver (dranet) ---
# Replaces the EFA device plugin: exposes the node's EFA interfaces as DRA
# devices under the DeviceClass efa.networking.k8s.aws (driver name "dra.net")
# and publishes a ResourceSlice carrying the dra.net/rdmaDevice attribute that
# the processor keys on. dranet and the EFA device plugin must not both manage
# EFA on the same node, so the device plugin is intentionally not installed here.

resource "helm_release" "aws_dranet" {
  name       = "aws-dranet"
  repository = "https://aws.github.io/eks-charts"
  chart      = "aws-dranet"
  version    = var.dranet_version
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
    { name = "clusterName", value = module.eks.cluster_name },
    { name = "region", value = var.region },
    { name = "otelContainerInsights.enabled", value = "true" },
  ]

  depends_on = [
    module.eks,
    null_resource.kubectl,
    data.external.clone_helm_chart,
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

resource "null_resource" "restart_pods" {
  depends_on = [null_resource.update_image]
  triggers   = { timestamp = timestamp() }
  provisioner "local-exec" {
    command = <<-EOT
      kubectl -n amazon-cloudwatch rollout restart daemonset/cloudwatch-agent
      kubectl -n amazon-cloudwatch rollout restart deployment/cloudwatch-agent-cluster-scraper 2>/dev/null || true
      kubectl -n amazon-cloudwatch rollout status daemonset/cloudwatch-agent --timeout=120s
    EOT
  }
}

# --- efaburn workload (DRA) ---
# efaburn claims 1 EFA via a ResourceClaimTemplate, so on a 2-EFA node exactly
# one device is claimed and the other stays unclaimed — exercising both the
# claimed->pod and unclaimed->no-pod correlation paths.

resource "null_resource" "efaburn" {
  depends_on = [module.eks, null_resource.kubectl, helm_release.aws_dranet]
  provisioner "local-exec" {
    command = <<-EOT
      cat <<'EOF' | kubectl apply -f -
      apiVersion: resource.k8s.io/v1
      kind: ResourceClaimTemplate
      metadata:
        name: efa-claim-template
        namespace: default
      spec:
        spec:
          devices:
            requests:
            - name: efa-device
              exactly:
                deviceClassName: efa.networking.k8s.aws
                count: 1
      ---
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: efaburn
        namespace: default
      spec:
        replicas: 1
        selector:
          matchLabels:
            app: efaburn
        template:
          metadata:
            labels:
              app: efaburn
              ci-test.example.com/pod-color: teal
          spec:
            nodeSelector:
              ci-test.example.com/multi-efa-dra: "true"
            resourceClaims:
            - name: efa-device
              resourceClaimTemplateName: efa-claim-template
            containers:
            - name: efaburn
              image: ${var.efaburn_image}
              resources:
                claims:
                - name: efa-device
                requests:
                  memory: 8000Mi
              securityContext:
                allowPrivilegeEscalation: false
                runAsNonRoot: true
                runAsUser: 1000
      EOF
    EOT
  }
}

# --- Test runner ---

resource "null_resource" "validator" {
  depends_on = [
    null_resource.restart_pods,
    null_resource.efaburn,
  ]

  triggers = { always_run = timestamp() }

  provisioner "local-exec" {
    command = <<-EOT
      echo "Running OTEL Multi-EFA DRA cluster integration tests"
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
