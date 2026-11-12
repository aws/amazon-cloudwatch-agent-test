# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
# SPDX-License-Identifier: MIT

# The chart with otlp.enabled=false, next to OTel Container Insights: the node
# agent must serve no OTLP endpoint. An ordinary pod tries to send OTLP through
# the cloudwatch-agent Service and logs the result, which test/otel/otlp_disabled
# reads.

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

  sender_script = "${path.module}/../../../otlp_service_sender.sh"
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
  node_group_name = "cwagent-otlp-off-${module.common.testing_id}"
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

  depends_on = [
    aws_iam_role_policy_attachment.node_AmazonEC2ContainerRegistryReadOnly,
    aws_iam_role_policy_attachment.node_AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy,
  ]
}

# EKS Node IAM Role
resource "aws_iam_role" "node_role" {
  name = "cwagent-otlp-off-eks-Worker-Role-${module.common.testing_id}"
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

# Pod Identity IAM Role. CloudWatchAgentServerPolicy is the policy customers
# are told to use, and covers the OTLP metrics, logs and traces exports.
resource "aws_iam_role" "pod_identity_role" {
  name = "cwagent-otlp-off-pod-identity-${module.common.testing_id}"
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
    set -e
    rm -rf ./helm-charts
    git clone -b ${var.helm_chart_branch} ${var.helm_chart_repo} ./helm-charts >&2
    # A chart without the OTLP endpoint would install fine and fail every
    # test; stop here with the reason instead.
    if ! grep -qE '^otlp:' ./helm-charts/charts/amazon-cloudwatch-observability/values.yaml; then
      echo "${var.helm_chart_repo}@${var.helm_chart_branch} has no top-level otlp block in values.yaml" >&2
      exit 1
    fi
    echo '{"status":"ready"}'
  EOT
  ]
}

# The chart defaults with the OTLP endpoint turned off, plus OTel Container
# Insights, which must not bring an OTLP receiver back.
resource "helm_release" "aws_observability" {
  name             = "amazon-cloudwatch-observability"
  chart            = "./helm-charts/charts/amazon-cloudwatch-observability"
  namespace        = "amazon-cloudwatch"
  create_namespace = true

  set = [
    { name = "clusterName", value = aws_eks_cluster.this.name },
    { name = "region", value = var.region },
    { name = "otelContainerInsights.enabled", value = "true" },
    { name = "otlp.enabled", value = "false" },
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
      set -e
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
      set -e
      kubectl -n amazon-cloudwatch rollout restart daemonset/cloudwatch-agent
      kubectl -n amazon-cloudwatch rollout restart deployment/cloudwatch-agent-cluster-scraper 2>/dev/null || true
      kubectl -n amazon-cloudwatch rollout status daemonset/cloudwatch-agent --timeout=120s
      kubectl -n amazon-cloudwatch rollout status deployment/cloudwatch-agent-cluster-scraper --timeout=120s 2>/dev/null || true
    EOT
  }
}

# --- OTLP probe ---
# An ordinary pod that sends OTLP to the default endpoint,
# http://cloudwatch-agent.amazon-cloudwatch:4318, and logs the HTTP status of
# every post (000 when the connection fails).

resource "null_resource" "senders" {
  depends_on = [null_resource.restart_pods]
  provisioner "local-exec" {
    command = <<-EOT
      set -e
      kubectl create namespace otlp-e2e --dry-run=client -o yaml | kubectl apply -f -
      kubectl -n otlp-e2e create configmap otlp-e2e-sender \
        --from-file=sender.sh=${local.sender_script} --dry-run=client -o yaml | kubectl apply -f -
      cat <<'EOF' | kubectl apply -f -
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: otlp-e2e-probe
        namespace: otlp-e2e
      spec:
        replicas: 1
        selector:
          matchLabels:
            app: otlp-e2e-probe
        template:
          metadata:
            labels:
              app: otlp-e2e-probe
          spec:
            containers:
            - name: sender
              image: curlimages/curl:8.8.0
              command: ["sh", "/sender/sender.sh"]
              env:
              - name: OTLP_CASE
                value: disabled
              resources:
                requests:
                  cpu: 10m
                  memory: 16Mi
              volumeMounts:
              - name: sender
                mountPath: /sender
            volumes:
            - name: sender
              configMap:
                name: otlp-e2e-sender
      EOF
      kubectl -n otlp-e2e rollout status deployment/otlp-e2e-probe --timeout=5m
    EOT
  }
}

# --- Diagnostics: agent and sender state in the job output, so a failed run
# can be debugged after the cluster is destroyed ---

resource "null_resource" "agent_diagnostics" {
  depends_on = [null_resource.senders]
  triggers   = { always_run = timestamp() }
  provisioner "local-exec" {
    command = <<-EOT
      sleep 30
      kubectl -n amazon-cloudwatch get pods,svc -o wide || true
      kubectl -n otlp-e2e get pods -o wide || true
      kubectl -n otlp-e2e logs deployment/otlp-e2e-probe --tail=6 --prefix || true
      kubectl -n amazon-cloudwatch logs ds/cloudwatch-agent --tail=100 --prefix || true
    EOT
  }
}

# --- Test runner ---

resource "null_resource" "validator" {
  depends_on = [null_resource.senders, null_resource.agent_diagnostics]

  triggers = { always_run = timestamp() }

  provisioner "local-exec" {
    command = <<-EOT
      echo "Running OTLP endpoint disabled cluster integration tests"
      cd ../../../..

      echo "Waiting 3 minutes, so that telemetry the agent should not export would have arrived..."
      sleep 180

      go test -tags integration -timeout 1h -v ${var.test_dir} \
        -eksClusterName=${aws_eks_cluster.this.name} \
        -computeType=EKS \
        -eksDeploymentStrategy=DAEMON \
        -region=${var.region}
    EOT
  }
}
