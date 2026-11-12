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

  # Node labels kept or removed by the nodeLabels filter. The two
  # karpenter.k8s.aws labels are fixtures, and the cluster runs no Karpenter.
  # Their domain is not kubernetes.io or k8s.io, so the kubelet can set them.
  node_labels = {
    "filters-e2e-node-keep"                = "v"
    "filters-e2e-node-drop"                = "v"
    "filters-e2e.example.com/drop-a"       = "v"
    "filters-e2e.example.com/drop-b"       = "v"
    "karpenter.k8s.aws/instance-gpu-name"  = "t4"
    "karpenter.k8s.aws/instance-gpu-count" = "1"
  }

  # One workload per namespace. The namespaces filter keeps the first two.
  # filters-exact-other starts with the exact include entry filters-exact, so
  # it is kept only if the exact entry matches as a prefix.
  namespaces = ["filters-keep-a", "filters-exact", "filters-keep-secret-b", "filters-exact-other"]

  # Namespaces of the stand-in inference servers: one kept and one dropped by
  # the namespaces filter.
  inference_namespaces = ["filters-keep-a", "filters-keep-secret-b"]

  # Run by the OTLP sender container of each test workload.
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
    set -e
    rm -rf ./helm-charts
    git clone -b ${var.helm_chart_branch} ${var.helm_chart_repo} ./helm-charts >&2
    # The OTLP cases need the chart's OTLP endpoint; a chart without it would
    # install fine and fail them. Stop here with the reason instead.
    if ! grep -qE '^otlp:' ./helm-charts/charts/amazon-cloudwatch-observability/values.yaml; then
      echo "${var.helm_chart_repo}@${var.helm_chart_branch} has no top-level otlp block in values.yaml" >&2
      exit 1
    fi
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
  # Metrics and logs share each list. metricNames applies to metrics only.
  # pod-template-hash is on the built-in list (recommendedExclusions, on by
  # default), but podLabels.include names it, so metrics and logs keep it.
  # nodeLabels.include has "*", so the agent collects every node label, and
  # the excludes and the built-in list remove the rest. Both karpenter.k8s.aws
  # fixture labels are inside the built-in prefix
  # karpenter.k8s.aws/instance-gpu-. nodeLabels.include names
  # instance-gpu-name, so the chart keeps it and deletes instance-gpu-count
  # with the rest of the prefix in one transform. podLabels.include has no
  # "*", so a pod label outside it is never collected.
  # metricNames.include names one vLLM and one Knative data plane metric, so
  # the namespaces filter alone decides whether they arrive. Both solutions
  # are on by default. The values set them so that the test does not depend
  # on the default.
  # metricNames.include names otlp_e2e_requests, one of the two counters the
  # OTLP sender container sends, so the other one, otlp_e2e_unlisted, and the
  # span metrics computed from the sender's spans are dropped.
  # filters-e2e.example.com/keep* is the only include entry that covers the pod
  # label filters-e2e.example.com/keep-x, so it tests a dotted prefix include.
  # The pod labels app-extra and filters-e2e-keep-secret-x and the metric
  # node_load15 start with an exact entry, so they test that exact entries do
  # not match as prefixes.
  values = [
    <<-EOT
      otelContainerInsights:
        solutions:
          vllm:
            enabled: true
          knative:
            dataPlane:
              enabled: true
      otelFilters:
        namespaces:
          include: ["filters-keep-*", "filters-exact"]
          exclude: ["filters-keep-secret*"]
        metricNames:
          include: ["container_*", "kube_pod_info", "k8s.pod.*", "node_load1", "vllm:num_requests_running", "revision_request_count", "otlp_e2e_requests"]
          exclude: ["container_network_*", "container_memory_usage_bytes"]
        podLabels:
          include: ["app", "filters-e2e-keep*", "filters-e2e.example.com/keep*", "pod-template-hash"]
          exclude: ["filters-e2e-keep-secret"]
        nodeLabels:
          include: ["*", "karpenter.k8s.aws/instance-gpu-name"]
          exclude: ["filters-e2e-node-drop", "filters-e2e.example.com/*"]
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

# --- Test workloads: one logging deployment per namespace, one pod per node ---
# Each pod also runs an OTLP sender container, which posts OTLP through the
# cloudwatch-agent Service (terraform/otlp_service_sender.sh), so the OTLP
# endpoint's telemetry from each namespace carries the same pod labels as the
# Container Insights telemetry.

resource "null_resource" "workloads" {
  for_each   = toset(local.namespaces)
  depends_on = [aws_eks_node_group.this, null_resource.kubectl]
  provisioner "local-exec" {
    command = <<-EOT
      set -e
      kubectl create namespace ${each.key} --dry-run=client -o yaml | kubectl apply -f -
      kubectl -n ${each.key} create configmap otlp-e2e-sender \
        --from-file=sender.sh=${local.sender_script} --dry-run=client -o yaml | kubectl apply -f -
      cat <<'EOF' | kubectl apply -f -
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: filters-e2e
        namespace: ${each.key}
      spec:
        # One pod on each of the 2 nodes, so every node agent sees a kept and a dropped namespace.
        replicas: 2
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
              filters-e2e.example.com/keep-x: v
              filters-e2e-keep-secret-x: v
              app-extra: v
          spec:
            affinity:
              podAntiAffinity:
                requiredDuringSchedulingIgnoredDuringExecution:
                - labelSelector:
                    matchLabels:
                      app: filters-e2e
                  topologyKey: kubernetes.io/hostname
            containers:
            - name: logger
              image: busybox:1.36
              command: ["sh", "-c", "while true; do echo otel-filters-e2e ${each.key}; sleep 5; done"]
              resources:
                requests:
                  cpu: 10m
                  memory: 16Mi
            - name: otlp-sender
              image: curlimages/curl:8.8.0
              command: ["sh", "/sender/sender.sh"]
              env:
              - name: OTLP_CASE
                value: filters
              - name: OTLP_METRICS
                value: otlp_e2e_requests otlp_e2e_unlisted
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
      kubectl -n ${each.key} rollout status deployment/filters-e2e --timeout=5m
    EOT
  }
}

# --- Stand-in inference servers: vLLM and Knative data plane metrics ---
# Each pod looks like a KServe predictor in Serverless mode. The vLLM scrape
# keeps the kserve-container container of a pod with the
# serving.kserve.io/inferenceservice label and component=predictor, on its
# port. The Knative data plane scrape keeps the container port named
# http-usermetric of a pod with the serving.knative.dev/revision label. Each
# container is nginx serving a fixed Prometheus text file at /metrics, with
# the text/plain content type that the Prometheus scraper needs. One pod runs
# on each node, so every node agent scrapes a kept and a dropped namespace.

# null_resource.workloads creates the namespaces.

resource "null_resource" "inference_stubs" {
  for_each   = toset(local.inference_namespaces)
  depends_on = [null_resource.workloads]
  provisioner "local-exec" {
    command = <<-EOT
      cat <<'EOF' | kubectl apply -f -
      apiVersion: v1
      kind: ConfigMap
      metadata:
        name: filters-e2e-inference
        namespace: ${each.key}
      data:
        vllm.conf: |
          events {}
          http {
            access_log off;
            default_type "text/plain; version=0.0.4";
            server {
              listen 8080;
              root /stub/vllm;
            }
          }
        knative.conf: |
          events {}
          http {
            access_log off;
            default_type "text/plain; version=0.0.4";
            server {
              listen 9091;
              root /stub/knative;
            }
          }
        vllm-metrics: |
          # HELP vllm:num_requests_running Number of requests in model execution batches.
          # TYPE vllm:num_requests_running gauge
          vllm:num_requests_running{model_name="filters-e2e"} 1
        knative-metrics: |
          # HELP revision_request_count The number of requests that are routed to queue-proxy.
          # TYPE revision_request_count counter
          revision_request_count{response_code="200"} 1
      ---
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: filters-e2e-inference
        namespace: ${each.key}
      spec:
        replicas: 2
        selector:
          matchLabels:
            app: filters-e2e-inference
        template:
          metadata:
            labels:
              app: filters-e2e-inference
              component: predictor
              serving.kserve.io/inferenceservice: filters-e2e
              serving.knative.dev/revision: filters-e2e-00001
          spec:
            affinity:
              podAntiAffinity:
                requiredDuringSchedulingIgnoredDuringExecution:
                - labelSelector:
                    matchLabels:
                      app: filters-e2e-inference
                  topologyKey: kubernetes.io/hostname
            containers:
            - name: kserve-container
              image: nginx:1.27-alpine
              command: ["nginx", "-c", "/stub/vllm.conf", "-g", "daemon off;"]
              ports:
              - containerPort: 8080
              resources:
                requests:
                  cpu: 10m
                  memory: 16Mi
              volumeMounts:
              - name: stub
                mountPath: /stub
            - name: queue-proxy
              image: nginx:1.27-alpine
              command: ["nginx", "-c", "/stub/knative.conf", "-g", "daemon off;"]
              ports:
              - name: http-usermetric
                containerPort: 9091
              resources:
                requests:
                  cpu: 10m
                  memory: 16Mi
              volumeMounts:
              - name: stub
                mountPath: /stub
            volumes:
            - name: stub
              configMap:
                name: filters-e2e-inference
                items:
                - key: vllm.conf
                  path: vllm.conf
                - key: knative.conf
                  path: knative.conf
                - key: vllm-metrics
                  path: vllm/metrics
                - key: knative-metrics
                  path: knative/metrics
      EOF
      kubectl -n ${each.key} rollout status deployment/filters-e2e-inference --timeout=5m
    EOT
  }
}

# --- Host log writer: a marker line in /var/log/messages on every node ---
# The AL2023 nodes have no /var/log/messages by default, so the host logs
# pipeline has no data without it. The namespace is not in the namespaces
# include list, so its own metrics and logs are dropped.

resource "null_resource" "host_log_writer" {
  depends_on = [aws_eks_node_group.this, null_resource.kubectl]
  provisioner "local-exec" {
    command = <<-EOT
      cat <<'EOF' | kubectl apply -f -
      apiVersion: v1
      kind: Namespace
      metadata:
        name: filters-host
      ---
      apiVersion: apps/v1
      kind: DaemonSet
      metadata:
        name: filters-e2e-host
        namespace: filters-host
      spec:
        selector:
          matchLabels:
            app: filters-e2e-host
        template:
          metadata:
            labels:
              app: filters-e2e-host
          spec:
            containers:
            - name: writer
              image: busybox:1.36
              command: ["sh", "-c", "while true; do echo otel-filters-e2e host >> /host/messages; sleep 5; done"]
              resources:
                requests:
                  cpu: 10m
                  memory: 16Mi
              volumeMounts:
              - name: messages
                mountPath: /host/messages
            volumes:
            - name: messages
              hostPath:
                path: /var/log/messages
                type: FileOrCreate
      EOF
      kubectl -n filters-host rollout status daemonset/filters-e2e-host --timeout=5m
    EOT
  }
}

# --- Test runner ---

resource "null_resource" "validator" {
  depends_on = [
    null_resource.restart_pods,
    null_resource.workloads,
    null_resource.inference_stubs,
    null_resource.host_log_writer,
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
