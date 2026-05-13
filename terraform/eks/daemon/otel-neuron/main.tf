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

# --- Node Groups ---

# Standard node group (operator/CoreDNS)

resource "aws_launch_template" "node" {
  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required"
    http_put_response_hop_limit = 2
  }
}

resource "aws_eks_node_group" "standard" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "cwagent-otel-standard-${module.common.testing_id}"
  node_role_arn   = aws_iam_role.node_role.arn
  subnet_ids      = module.basic_components.public_subnet_ids

  scaling_config {
    desired_size = 1
    max_size     = 1
    min_size     = 1
  }

  ami_type       = "AL2023_x86_64_STANDARD"
  capacity_type  = "ON_DEMAND"
  instance_types = ["t3.medium"]

  launch_template {
    id      = aws_launch_template.node.id
    version = aws_launch_template.node.latest_version
  }

  depends_on = [
    aws_iam_role_policy_attachment.node_AmazonEC2ContainerRegistryReadOnly,
    aws_iam_role_policy_attachment.node_AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy,
  ]
}

# Neuron workload node group (runs neuron-burn-core)
resource "aws_eks_node_group" "neuron_workload" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "cwagent-otel-neuron-workload-${module.common.testing_id}"
  node_role_arn   = aws_iam_role.node_role.arn
  subnet_ids      = module.basic_components.public_subnet_ids

  scaling_config {
    desired_size = 1
    max_size     = 1
    min_size     = 1
  }

  ami_type       = var.ami_type
  capacity_type  = "ON_DEMAND"
  instance_types = [var.instance_type]

  launch_template {
    id      = aws_launch_template.node.id
    version = aws_launch_template.node.latest_version
  }

  labels = {
    "aws.amazon.com/neuron.present"  = "true"
    "ci-test.example.com/node-color" = "red"
  }

  taint {
    key    = "aws.amazon.com/neuron"
    value  = "true"
    effect = "NO_SCHEDULE"
  }

  depends_on = [
    aws_iam_role_policy_attachment.node_AmazonEC2ContainerRegistryReadOnly,
    aws_iam_role_policy_attachment.node_AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy,
  ]
}

# Neuron idle node group (no workload, still emits neuron-monitor metrics)
resource "aws_eks_node_group" "neuron_idle" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "cwagent-otel-neuron-idle-${module.common.testing_id}"
  node_role_arn   = aws_iam_role.node_role.arn
  subnet_ids      = module.basic_components.public_subnet_ids

  scaling_config {
    desired_size = 1
    max_size     = 1
    min_size     = 1
  }

  ami_type       = var.ami_type
  capacity_type  = "ON_DEMAND"
  instance_types = [var.instance_type]

  launch_template {
    id      = aws_launch_template.node.id
    version = aws_launch_template.node.latest_version
  }

  labels = {
    "aws.amazon.com/neuron.present"  = "true"
    "ci-test.example.com/node-color" = "red"
  }

  taint {
    key    = "aws.amazon.com/neuron"
    value  = "true"
    effect = "NO_SCHEDULE"
  }

  depends_on = [
    aws_iam_role_policy_attachment.node_AmazonEC2ContainerRegistryReadOnly,
    aws_iam_role_policy_attachment.node_AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy,
  ]
}

# Multi-device Neuron node group (inf2.24xlarge = 6 devices × 2 cores = 12 cores)
# Validates multi-device per-node attribution and cardinality.
# Pinned to usw2-az3 for inf2.24xlarge capacity availability.
data "aws_subnet" "multi_device_subnet" {
  for_each = toset(module.basic_components.public_subnet_ids)
  id       = each.value
}

locals {
  multi_device_az_id   = "usw2-az3"
  multi_device_subnets = [for s in data.aws_subnet.multi_device_subnet : s.id if s.availability_zone_id == local.multi_device_az_id]
}

resource "aws_eks_node_group" "neuron_multi_device" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "cwagent-otel-neuron-multi-${module.common.testing_id}"
  node_role_arn   = aws_iam_role.node_role.arn
  subnet_ids      = local.multi_device_subnets

  scaling_config {
    desired_size = 1
    max_size     = 1
    min_size     = 1
  }

  ami_type       = var.ami_type
  capacity_type  = "ON_DEMAND"
  instance_types = [var.multi_device_instance_type]

  launch_template {
    id      = aws_launch_template.node.id
    version = aws_launch_template.node.latest_version
  }

  labels = {
    "aws.amazon.com/neuron.present"  = "true"
    "ci-test.example.com/node-color" = "purple"
    "ci-test.example.com/node-tier"  = "multi-device"
  }

  taint {
    key    = "aws.amazon.com/neuron"
    value  = "true"
    effect = "NO_SCHEDULE"
  }

  depends_on = [
    aws_iam_role_policy_attachment.node_AmazonEC2ContainerRegistryReadOnly,
    aws_iam_role_policy_attachment.node_AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy,
  ]
}

# --- IAM ---

resource "aws_iam_role" "node_role" {
  name = "cwagent-otel-neuron-Worker-Role-${module.common.testing_id}"
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
  name = "cwagent-otel-neuron-pod-identity-${module.common.testing_id}"
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
  depends_on = [
    aws_eks_cluster.this,
    aws_eks_node_group.standard,
    aws_eks_node_group.neuron_workload,
    aws_eks_node_group.neuron_idle,
    aws_eks_node_group.neuron_multi_device,
  ]
  provisioner "local-exec" {
    command = "${local.aws_eks} update-kubeconfig --name ${aws_eks_cluster.this.name}"
  }
}

# --- Neuron device plugin via Helm ---

resource "helm_release" "neuron_device_plugin" {
  depends_on = [
    aws_eks_node_group.neuron_workload,
    aws_eks_node_group.neuron_idle,
    aws_eks_node_group.neuron_multi_device,
    null_resource.kubectl,
  ]

  name       = "neuron-helm-chart"
  repository = "oci://public.ecr.aws/neuron"
  chart      = "neuron-helm-chart"
  namespace  = "kube-system"

  set = [
    { name = "npd.enabled", value = "false" },
    { name = "scheduler.enabled", value = "false" },
  ]
}

# --- neuron-burn-core Deployment ---

resource "null_resource" "neuron_burn_core" {
  depends_on = [
    helm_release.neuron_device_plugin,
    null_resource.kubectl,
  ]
  provisioner "local-exec" {
    command = <<-EOT
      cat <<'EOF' | kubectl apply -f -
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: neuron-burn-core
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
            app: neuron-burn-core
        template:
          metadata:
            labels:
              app: neuron-burn-core
              neuron-test: "true"
              ci-test.example.com/pod-color: orange
          spec:
            tolerations:
            - key: aws.amazon.com/neuron
              operator: Exists
              effect: NoSchedule
            affinity:
              podAntiAffinity:
                requiredDuringSchedulingIgnoredDuringExecution:
                - labelSelector:
                    matchExpressions:
                    - key: neuron-test
                      operator: In
                      values: ["true"]
                  topologyKey: kubernetes.io/hostname
            nodeSelector:
              node.kubernetes.io/instance-type: ${var.instance_type}
            containers:
            - name: neuron-burn
              image: public.ecr.aws/neuron/pytorch-inference-neuronx:2.1.2-neuronx-py310-sdk2.20.2-ubuntu20.04
              command: ["python3", "-c"]
              args:
              - |
                import torch
                import torch_neuronx
                import time
                print("Compiling neuron trace (this takes a minute)...")
                x = torch.randn(256, 256)
                model = torch.nn.Linear(256, 256, bias=False)
                traced = torch_neuronx.trace(model, x)
                print("Trace compiled. Starting burn loop...")
                iteration = 0
                while True:
                    start = time.time()
                    for _ in range(1000):
                        _ = traced(x)
                    elapsed = time.time() - start
                    iteration += 1
                    print(f"Iteration {iteration}: 1000 inferences in {elapsed:.2f}s")
              resources:
                limits:
                  aws.amazon.com/neuroncore: "1"
                requests:
                  cpu: "1"
                  memory: 4Gi
      EOF
    EOT
  }
}

# --- neuron-burn-multi-device Deployment (inf2.24xlarge, uses multiple devices) ---

resource "null_resource" "neuron_burn_multi_device" {
  depends_on = [
    helm_release.neuron_device_plugin,
    null_resource.kubectl,
  ]
  provisioner "local-exec" {
    command = <<-EOT
      cat <<'EOF' | kubectl apply -f -
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: neuron-burn-multi
        namespace: default
      spec:
        replicas: 1
        revisionHistoryLimit: 2
        progressDeadlineSeconds: 600
        strategy:
          type: RollingUpdate
          rollingUpdate:
            maxSurge: 0
            maxUnavailable: 1
        selector:
          matchLabels:
            app: neuron-burn-multi
        template:
          metadata:
            labels:
              app: neuron-burn-multi
              neuron-test: "true"
              ci-test.example.com/pod-color: violet
          spec:
            tolerations:
            - key: aws.amazon.com/neuron
              operator: Exists
              effect: NoSchedule
            nodeSelector:
              node.kubernetes.io/instance-type: ${var.multi_device_instance_type}
            containers:
            - name: neuron-burn
              image: public.ecr.aws/neuron/pytorch-inference-neuronx:2.1.2-neuronx-py310-sdk2.20.2-ubuntu20.04
              command: ["python3", "-c"]
              args:
              - |
                import torch, torch_neuronx, time, os
                # Device allocation is controlled by the resource limit
                # aws.amazon.com/neuron: "1" (1 whole device = 2 cores).
                # NEURON_CORES is informational only — used for the log message
                # below so we can see how many cores the Python process sees.
                num_cores = int(os.environ.get("NEURON_CORES", "2"))
                print(f"Compiling neuron trace for {num_cores} cores...")
                x = torch.randn(256, 256)
                model = torch.nn.Linear(256, 256, bias=False)
                traced = torch_neuronx.trace(model, x)
                print("Trace compiled. Starting burn loop...")
                iteration = 0
                while True:
                    start = time.time()
                    for _ in range(1000):
                        _ = traced(x)
                    elapsed = time.time() - start
                    iteration += 1
                    print(f"Iteration {iteration}: 1000 inferences in {elapsed:.2f}s")
              env:
              - name: NEURON_CORES
                value: "1"
              resources:
                limits:
                  aws.amazon.com/neuron: "1"
                requests:
                  cpu: "1"
                  memory: 4Gi
      EOF
    EOT
  }
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

# --- Patch agent image (DaemonSet + cluster-scraper) ---

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

# --- Wait for neuron-monitor pods to be ready ---

resource "null_resource" "wait_neuron_monitor" {
  depends_on = [null_resource.restart_pods, null_resource.neuron_burn_core, null_resource.neuron_burn_multi_device]
  triggers   = { timestamp = timestamp() }
  provisioner "local-exec" {
    command = <<-EOT
      echo "Waiting for neuron-monitor pods to be ready..."
      READY=0
      for i in $(seq 1 30); do
        READY=$(kubectl -n amazon-cloudwatch get pods -l app.kubernetes.io/name=neuron-monitor --no-headers 2>/dev/null | grep -c "Running" || true)
        if [ "$READY" -ge 3 ]; then
          echo "neuron-monitor pods ready ($READY running)"
          break
        fi
        echo "Attempt $i: $READY neuron-monitor pods running, waiting..."
        sleep 10
      done
      if [ "$READY" -lt 3 ]; then
        echo "ERROR: only $READY neuron-monitor pods ready after 5 minutes"
        exit 1
      fi
    EOT
  }
}

# --- Test runner ---

resource "null_resource" "validator" {
  depends_on = [null_resource.wait_neuron_monitor]
  triggers   = { always_run = timestamp() }
  provisioner "local-exec" {
    command = <<-EOT
      echo "Running OTEL Neuron cluster integration tests"
      cd ../../../..

      echo "Waiting for Neuron runtime to initialize (model compile + burn loop start)..."
      echo "This can take up to 15 minutes on a cold start due to PyTorch-Neuron image pull + trace compilation."
      echo "Readiness signal: 'Iteration N' log lines from the active burn loop."
      CORE_READY=0
      MULTI_READY=0
      # 90 iterations × 10s = 15 minutes max.
      for i in $(seq 1 90); do
        # Match "Iteration" in the tail — the burn loop emits one per second once
        # the trace is compiled. This is a more robust readiness signal than
        # grepping for "Trace compiled" which scrolls off the tail quickly.
        CORE_READY=$(kubectl logs -n default -l app=neuron-burn-core --tail=5 2>/dev/null | grep -c "^Iteration " || true)
        MULTI_READY=$(kubectl logs -n default -l app=neuron-burn-multi --tail=5 2>/dev/null | grep -c "^Iteration " || true)
        if [ "$CORE_READY" -gt 0 ] && [ "$MULTI_READY" -gt 0 ]; then
          echo "Neuron runtime active on both burn workloads (after $((i*10))s)"
          break
        fi
        # Every 60s dump pod status so we can see image-pull vs crash vs runtime-init.
        if [ $((i % 6)) -eq 0 ]; then
          echo "--- Attempt $i ($((i*10))s elapsed): core_iter_lines=$CORE_READY multi_iter_lines=$MULTI_READY ---"
          kubectl get pods -n default -l neuron-test=true -o wide 2>&1 | head -10 || true
        fi
        sleep 10
      done
      if [ "$CORE_READY" -eq 0 ] || [ "$MULTI_READY" -eq 0 ]; then
        echo "ERROR: Neuron burn loop not active after 15 minutes (core_iter=$CORE_READY multi_iter=$MULTI_READY)"
        echo "=== Final pod status ==="
        kubectl get pods -n default -l neuron-test=true -o wide 2>&1 || true
        echo "=== burn-core events ==="
        kubectl describe pod -n default -l app=neuron-burn-core 2>&1 | tail -40 || true
        echo "=== burn-multi events ==="
        kubectl describe pod -n default -l app=neuron-burn-multi 2>&1 | tail -40 || true
        echo "=== burn-core logs ==="
        kubectl logs -n default -l app=neuron-burn-core --tail=40 2>&1 || true
        echo "=== burn-multi logs ==="
        kubectl logs -n default -l app=neuron-burn-multi --tail=40 2>&1 || true
        exit 1
      fi

      echo "=== Burn workload pod status ==="
      kubectl get pods -n default -l neuron-test=true -o wide

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
