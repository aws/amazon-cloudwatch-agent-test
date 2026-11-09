# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
# SPDX-License-Identifier: MIT

# The chart's OTLP endpoint with its default values, next to OTel Container
# Insights. Ordinary pods (no hostNetwork) send OTLP through the
# cloudwatch-agent Service and set no pod attributes, so the node agent has to
# find each sender from the connection's source IP. One sender per service
# identity case: see local.senders and test/otel/otlp.

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

  # A node label fixture, so the test can check that node labels reach OTLP
  # telemetry. kubernetes.io/os and kubernetes.io/arch come from the kubelet.
  node_labels = {
    "otlp-e2e-node" = "v"
  }

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
  node_group_name = "cwagent-otlp-${module.common.testing_id}"
  node_role_arn   = aws_iam_role.node_role.arn
  subnet_ids      = module.basic_components.public_subnet_ids

  # Two nodes, so that a sender on each node proves each node agent resolves
  # its own pods.
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
  name = "cwagent-otlp-eks-Worker-Role-${module.common.testing_id}"
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
  name = "cwagent-otlp-pod-identity-${module.common.testing_id}"
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

# The chart defaults, so the OTLP endpoint is on with its default ports and
# span metrics, plus OTel Container Insights to check that both run together.
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

# --- OTLP senders, one per service identity case ---
# Every sender but grpc runs terraform/otlp_service_sender.sh from a ConfigMap
# and posts OTLP/HTTP to the default endpoint,
# http://cloudwatch-agent.amazon-cloudwatch:4318. Each pod has one container,
# so the agent also sets k8s.container.name. The expected service.name of each
# case is in test/otel/otlp/otlp_test.go.
#
#   app        app.kubernetes.io/name and /version labels, no service.name sent;
#              two replicas, one per node. otelServiceIdentity.useAppLabels is
#              false by default, so the labels are not used: service.name is
#              the Deployment name and service.version is not set
#   annotation resource.opentelemetry.io/service.name and /service.version
#              annotations, which win over the Deployment name
#   explicit   the sender sends service.name, which wins over the Deployment name
#   unknown    the sender sends unknown_service:sh, which counts as not set,
#              so the Deployment name is used
#   daemonset  a DaemonSet with no labels or annotations for identity
#   cronjob    a CronJob with no labels or annotations for identity
#   grpc       telemetrygen sending metrics over OTLP/gRPC to
#              cloudwatch-agent.amazon-cloudwatch:4317, with
#              unknown_service:telemetrygen, so the Deployment name is used
#
# The senders start after the agents restart, so that the first records are
# not sent to an agent that is going away.

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
        name: otlp-e2e-app
        namespace: otlp-e2e
      spec:
        replicas: 2
        selector:
          matchLabels:
            app: otlp-e2e-app
        template:
          metadata:
            labels:
              app: otlp-e2e-app
              app.kubernetes.io/name: otlp-e2e-app-name
              app.kubernetes.io/version: "1.2.3"
          spec:
            affinity:
              podAntiAffinity:
                requiredDuringSchedulingIgnoredDuringExecution:
                - labelSelector:
                    matchLabels:
                      app: otlp-e2e-app
                  topologyKey: kubernetes.io/hostname
            containers:
            - name: sender
              image: curlimages/curl:8.8.0
              command: ["sh", "/sender/sender.sh"]
              env:
              - name: OTLP_CASE
                value: app
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
      ---
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: otlp-e2e-annotated
        namespace: otlp-e2e
      spec:
        replicas: 1
        selector:
          matchLabels:
            app: otlp-e2e-annotated
        template:
          metadata:
            labels:
              app: otlp-e2e-annotated
              app.kubernetes.io/name: otlp-e2e-label-loses
              app.kubernetes.io/version: "0.0.1"
            annotations:
              resource.opentelemetry.io/service.name: otlp-e2e-annotated-svc
              resource.opentelemetry.io/service.version: "9.9.9"
          spec:
            containers:
            - name: sender
              image: curlimages/curl:8.8.0
              command: ["sh", "/sender/sender.sh"]
              env:
              - name: OTLP_CASE
                value: annotation
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
      ---
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: otlp-e2e-explicit
        namespace: otlp-e2e
      spec:
        replicas: 1
        selector:
          matchLabels:
            app: otlp-e2e-explicit
        template:
          metadata:
            labels:
              app: otlp-e2e-explicit
              app.kubernetes.io/name: otlp-e2e-label-loses
          spec:
            containers:
            - name: sender
              image: curlimages/curl:8.8.0
              command: ["sh", "/sender/sender.sh"]
              env:
              - name: OTLP_CASE
                value: explicit
              - name: OTLP_SERVICE_NAME
                value: otlp-e2e-explicit-svc
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
      ---
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: otlp-e2e-unknown
        namespace: otlp-e2e
      spec:
        replicas: 1
        selector:
          matchLabels:
            app: otlp-e2e-unknown
        template:
          metadata:
            labels:
              app: otlp-e2e-unknown
          spec:
            containers:
            - name: sender
              image: curlimages/curl:8.8.0
              command: ["sh", "/sender/sender.sh"]
              env:
              - name: OTLP_CASE
                value: unknown
              - name: OTLP_SERVICE_NAME
                value: unknown_service:sh
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
      ---
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: otlp-e2e-grpc
        namespace: otlp-e2e
      spec:
        replicas: 1
        selector:
          matchLabels:
            app: otlp-e2e-grpc
        template:
          metadata:
            labels:
              app: otlp-e2e-grpc
          spec:
            containers:
            - name: sender
              image: ghcr.io/open-telemetry/opentelemetry-collector-contrib/telemetrygen:v0.150.0
              args:
              - metrics
              - --otlp-endpoint=cloudwatch-agent.amazon-cloudwatch:4317
              - --otlp-insecure
              - --metric-type=Sum
              - --otlp-metric-name=otlp_e2e_requests
              - '--telemetry-attributes=otlp_e2e_case="grpc"'
              - --service=unknown_service:telemetrygen
              - --rate=0.1
              - --duration=inf
              resources:
                requests:
                  cpu: 10m
                  memory: 16Mi
      ---
      apiVersion: apps/v1
      kind: DaemonSet
      metadata:
        name: otlp-e2e-ds
        namespace: otlp-e2e
      spec:
        selector:
          matchLabels:
            app: otlp-e2e-ds
        template:
          metadata:
            labels:
              app: otlp-e2e-ds
          spec:
            containers:
            - name: sender
              image: curlimages/curl:8.8.0
              command: ["sh", "/sender/sender.sh"]
              env:
              - name: OTLP_CASE
                value: daemonset
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
      ---
      apiVersion: batch/v1
      kind: CronJob
      metadata:
        name: otlp-e2e-cron
        namespace: otlp-e2e
      spec:
        schedule: "* * * * *"
        concurrencyPolicy: Forbid
        successfulJobsHistoryLimit: 1
        failedJobsHistoryLimit: 1
        jobTemplate:
          spec:
            backoffLimit: 0
            template:
              metadata:
                labels:
                  app: otlp-e2e-cron
              spec:
                restartPolicy: Never
                containers:
                - name: sender
                  image: curlimages/curl:8.8.0
                  command: ["sh", "/sender/sender.sh"]
                  env:
                  - name: OTLP_CASE
                    value: cronjob
                  # Three rounds 10s apart, so a round arrives after the
                  # agent's pod cache has the new pod's IP.
                  - name: OTLP_ITERATIONS
                    value: "3"
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
      for w in deployment/otlp-e2e-app deployment/otlp-e2e-annotated deployment/otlp-e2e-explicit \
        deployment/otlp-e2e-unknown deployment/otlp-e2e-grpc daemonset/otlp-e2e-ds; do
        kubectl -n otlp-e2e rollout status "$w" --timeout=5m
      done
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
      kubectl -n otlp-e2e logs deployment/otlp-e2e-app --tail=6 --prefix || true
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
      echo "Running OTLP endpoint cluster integration tests"
      cd ../../../..

      echo "Waiting 3 minutes for metrics, logs and traces to propagate..."
      sleep 180

      go test -tags integration -timeout 1h -v ${var.test_dir} \
        -eksClusterName=${aws_eks_cluster.this.name} \
        -computeType=EKS \
        -eksDeploymentStrategy=DAEMON \
        -region=${var.region}
    EOT
  }
}
