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

  # One neuron-burn pod per namespace, each with 1 of the node's 2
  # NeuronCores. otelFilters.namespaces.exclude has neuron-drop, so the chart
  # keeps the neuron-drop core's metrics and removes their pod, namespace and
  # container.
  neuron_burn_namespaces = ["neuron-keep", "neuron-drop"]

  # Node labels kept or removed by the nodeLabels filter.
  # neuron-filters-node-keep-extra starts with the exact include entry, so it
  # is kept only if the exact entry matches as a prefix.
  neuron_node_labels = {
    "aws.amazon.com/neuron.present"     = "true"
    "neuron-filters-node-keep"          = "v"
    "neuron-filters-node-keep-extra"    = "v"
    "neuron-filters-node-other"         = "v"
    "neuron-filters.example.com/keep-x" = "v"
    "neuron-filters.example.com/drop-a" = "v"
  }
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
  name = "cwagent-otel-neuron-filters-Worker-Role-${module.common.testing_id}"
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

# --- Neuron Node Group: 1x inf2.xlarge with 1 device and 2 NeuronCores ---

resource "aws_eks_node_group" "neuron" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "neuron-${module.common.testing_id}"
  node_role_arn   = aws_iam_role.node_role.arn
  subnet_ids      = module.basic_components.public_subnet_ids

  scaling_config {
    desired_size = 1
    max_size     = 1
    min_size     = 1
  }

  ami_type       = var.ami_type
  capacity_type  = "ON_DEMAND"
  disk_size      = 50
  instance_types = [var.instance_type]
  labels         = local.neuron_node_labels

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

# Pod Identity IAM Role
resource "aws_iam_role" "pod_identity_role" {
  name = "cwagent-otel-neuron-filters-pod-identity-${module.common.testing_id}"
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
  depends_on = [aws_eks_cluster.this, aws_eks_node_group.standard, aws_eks_node_group.neuron]
  provisioner "local-exec" {
    command = "${local.aws_eks} update-kubeconfig --name ${aws_eks_cluster.this.name}"
  }
}

# --- Neuron device plugin (Helm) ---

resource "helm_release" "neuron_device_plugin" {
  depends_on = [
    aws_eks_node_group.neuron,
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

# --- neuron-burn workloads: one pod per namespace, each with its own core ---
# The pod labels are kept or removed by the podLabels filter. app-extra and
# neuron-filters-keep-secret-x start with an exact entry, so they test that
# exact entries do not match as prefixes.

resource "null_resource" "neuron_burn" {
  for_each   = toset(local.neuron_burn_namespaces)
  depends_on = [helm_release.neuron_device_plugin, null_resource.kubectl]
  provisioner "local-exec" {
    command = <<-EOT
      set -e
      cat <<'EOF' | kubectl apply -f -
      apiVersion: v1
      kind: Namespace
      metadata:
        name: ${each.key}
      ---
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: neuron-burn
        namespace: ${each.key}
      spec:
        replicas: 1
        # A cold pull of the PyTorch-Neuron image can take more than the
        # default 600s.
        progressDeadlineSeconds: 1200
        selector:
          matchLabels:
            app: neuron-burn
        template:
          metadata:
            labels:
              app: neuron-burn
              app-extra: v
              neuron-filters-keep-a: v
              neuron-filters-keep-secret: v
              neuron-filters-keep-secret-x: v
              neuron-filters-keep-drop-x: v
              neuron-filters-other: v
          spec:
            tolerations:
            - key: aws.amazon.com/neuron
              operator: Exists
              effect: NoSchedule
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
              env:
              # The runtime tag defaults to the PID, which is 1 in every container.
              - name: NEURON_PROCESS_TAG
                valueFrom:
                  fieldRef:
                    fieldPath: metadata.name
              resources:
                limits:
                  aws.amazon.com/neuroncore: "1"
                requests:
                  cpu: "1"
                  memory: 4Gi
      EOF
      kubectl -n ${each.key} rollout status deployment/neuron-burn --timeout=20m
    EOT
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

  # Each label filter has an exact include, a prefix include and an exclude.
  # The pod label neuron-filters-keep-drop-x is inside the include prefix,
  # and the exclude prefix removes it. The node label
  # neuron-filters.example.com/drop-a is inside the dotted include prefix,
  # and the dotted exclude prefix removes it. The pod label
  # neuron-filters-other and the node labels neuron-filters-node-other and
  # aws.amazon.com/neuron.present are outside include, so the agent never
  # collects them.
  values = [
    <<-EOT
      otelFilters:
        namespaces:
          exclude: ["neuron-drop"]
        podLabels:
          include: ["app", "neuron-filters-keep*"]
          exclude: ["neuron-filters-keep-secret", "neuron-filters-keep-drop*"]
        nodeLabels:
          include: ["neuron-filters-node-keep", "neuron-filters.example.com/*"]
          exclude: ["neuron-filters.example.com/drop*"]
    EOT
  ]

  # No agent runs before both neuron-burn pods hold their cores. Otherwise a
  # core exports series without a pod before its pod starts, and they look
  # the same as the stripped neuron-drop core.
  depends_on = [
    aws_eks_addon.pod_identity_agent,
    helm_release.neuron_device_plugin,
    null_resource.kubectl,
    data.external.clone_helm_chart,
    null_resource.neuron_burn,
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
# The agents start only after both neuron-burn pods hold their cores (see
# helm_release.aws_observability), so every agent starts with both cores in
# use.

resource "null_resource" "restart_pods" {
  depends_on = [aws_eks_pod_identity_association.cloudwatch_agent, null_resource.update_image, null_resource.neuron_burn]
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
    null_resource.neuron_burn,
  ]

  triggers = { always_run = timestamp() }

  provisioner "local-exec" {
    command = <<-EOT
      echo "Running OTEL Neuron filters cluster integration tests"
      cd ../../../..

      echo "Waiting for neuron-monitor pods to be ready..."
      for i in $(seq 1 30); do
        READY=$(kubectl get pods -n amazon-cloudwatch -l app.kubernetes.io/name=neuron-monitor -o jsonpath='{.items[*].status.phase}' 2>/dev/null | tr ' ' '\n' | grep -c Running || true)
        if [ "$READY" -ge 1 ] 2>/dev/null; then break; fi
        sleep 20
      done

      # A core reports only once a runtime runs, after the trace compile.
      echo "Waiting for both neuron-burn runtimes to start..."
      for ns in ${join(" ", local.neuron_burn_namespaces)}; do
        for i in $(seq 1 90); do
          if kubectl logs -n $ns -l app=neuron-burn --tail=5 2>/dev/null | grep -q "^Iteration "; then break; fi
          if [ "$i" -eq 90 ]; then
            echo "ERROR: no neuron-burn runtime in $ns after 15 minutes"
            kubectl -n $ns describe pod -l app=neuron-burn | tail -40 || true
            kubectl -n $ns logs -l app=neuron-burn --tail=40 || true
            exit 1
          fi
          sleep 10
        done
      done

      echo "Waiting 3 minutes for metrics to propagate..."
      sleep 180

      go test -tags integration -timeout 1h -v ${var.test_dir} \
        -eksClusterName=${aws_eks_cluster.this.name} \
        -computeType=EKS \
        -eksDeploymentStrategy=DAEMON \
        -region=${var.region}
    EOT
  }
}
