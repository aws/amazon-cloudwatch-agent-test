# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
# SPDX-License-Identifier: MIT

# Neuron DRA-path integration test cluster. Mirrors otel-neuron but exposes Neuron
# devices via the AWS Neuron DRA driver (DeviceClass neuron.aws.com) instead of the
# device plugin, and the burn workload claims a device via a ResourceClaimTemplate.
# One Trainium node (trn1.2xlarge = 1 device x 2 cores) proves per-device/per-core
# DRA correlation: the claimed device's 2 cores attribute to the burn pod, and to
# no other pod. Trainium (not Inferentia): the Neuron DRA driver image 1.2.0 supports
# Trainium only and rejects inf1/inf2 at device discovery ("unsupported instance
# type"); trn1.2xlarge is the smallest/most-available Trainium instance.

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

# --- IAM ---

resource "aws_iam_role" "node_role" {
  name = "cwagent-otel-neuron-dra-Worker-Role-${module.common.testing_id}"
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

resource "aws_iam_role" "pod_identity_role" {
  name = "cwagent-otel-neuron-dra-pod-identity-${module.common.testing_id}"
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

# --- Node Groups ---

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

# Neuron (Trainium) node (trn1.2xlarge). No taint: the node is dedicated by
# being the only Neuron node, and the DRA driver DaemonSet + burn pod land here via
# scheduling/nodeSelector. Avoiding a taint keeps the Neuron DRA driver DaemonSet
# (whose default tolerations are not assumed) schedulable.
# Pinned to usw2-az4: trn1.2xlarge is offered only in usw2-az1/usw2-az4 in us-west-2.
data "aws_subnet" "az" {
  for_each = toset(module.basic_components.public_subnet_ids)
  id       = each.value
}

locals {
  neuron_az_id   = "usw2-az4"
  neuron_subnets = [for s in data.aws_subnet.az : s.id if s.availability_zone_id == local.neuron_az_id]
}

resource "aws_eks_node_group" "neuron_multi" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "neuron-multi-${module.common.testing_id}"
  node_role_arn   = aws_iam_role.node_role.arn
  subnet_ids      = local.neuron_subnets

  scaling_config {
    desired_size = 1
    max_size     = 1
    min_size     = 1
  }

  ami_type       = var.ami_type
  capacity_type  = "ON_DEMAND"
  disk_size      = 100
  instance_types = [var.instance_type]

  labels = {
    "aws.amazon.com/neuron.present"  = "true"
    "ci-test.example.com/node-color" = "purple"
  }

  depends_on = [
    aws_iam_role_policy_attachment.node_AmazonEC2ContainerRegistryReadOnly,
    aws_iam_role_policy_attachment.node_AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy,
  ]
}

# --- EKS Addon: Pod Identity agent ---

resource "aws_eks_addon" "pod_identity_agent" {
  depends_on   = [aws_eks_node_group.standard]
  cluster_name = aws_eks_cluster.this.name
  addon_name   = "eks-pod-identity-agent"
}

# --- Update kubeconfig ---

resource "null_resource" "kubectl" {
  depends_on = [aws_eks_cluster.this, aws_eks_node_group.standard, aws_eks_node_group.neuron_multi]
  provisioner "local-exec" {
    command = "${local.aws_eks} update-kubeconfig --name ${aws_eks_cluster.this.name}"
  }
}

# --- Neuron DRA driver (Helm) — replaces the device plugin ---
# DeviceClass neuron.aws.com, driver neuron.aws.com. devicePlugin disabled so it
# does not compete with the DRA driver for the same devices.

resource "helm_release" "neuron_dra_driver" {
  depends_on = [aws_eks_node_group.neuron_multi, null_resource.kubectl]

  name       = "neuron-helm-chart"
  repository = "oci://public.ecr.aws/neuron"
  chart      = "neuron-helm-chart"
  # The chart renders its own Namespace object for neuron-dra-driver (via
  # draDriver.namespaceOverride, no disable toggle) AND places the driver
  # DaemonSet/SA/RBAC there. So we install the release into the pre-existing
  # kube-system namespace (Helm needs the release namespace to exist to store the
  # release secret) with create_namespace=false, and let the chart create and own
  # neuron-dra-driver. Installing into neuron-dra-driver directly deadlocks: with
  # create_namespace=true the provider's namespace collides with the chart's
  # ("already exists"); with false Helm fails "namespace not found".
  namespace        = "kube-system"
  create_namespace = false
  version          = var.neuron_helm_chart_version != "" ? var.neuron_helm_chart_version : null

  set = [
    { name = "devicePlugin.enabled", value = "false" },
    { name = "npd.enabled", value = "false" },
    { name = "scheduler.enabled", value = "false" },
    { name = "draDriver.enabled", value = "true" },
  ]
}

# --- neuron-burn-dra Deployment: claims 1 whole Neuron device via DRA ---

resource "null_resource" "neuron_burn_dra" {
  depends_on = [helm_release.neuron_dra_driver, null_resource.kubectl]
  # Re-apply the manifest (kubectl apply is idempotent) if the instance type
  # changes, so the burn pod's node-type nodeSelector tracks var.instance_type.
  triggers = {
    instance_type = var.instance_type
    cluster       = aws_eks_cluster.this.name
  }
  provisioner "local-exec" {
    command = <<-EOT
      cat <<'EOF' | kubectl apply -f -
      apiVersion: resource.k8s.io/v1
      kind: ResourceClaimTemplate
      metadata:
        name: neuron-1
        namespace: default
      spec:
        spec:
          devices:
            requests:
            - name: neuron
              exactly:
                deviceClassName: neuron.aws.com
                count: 1
      ---
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: neuron-burn-dra
        namespace: default
      spec:
        replicas: 1
        revisionHistoryLimit: 2
        progressDeadlineSeconds: 600
        selector:
          matchLabels:
            app: neuron-burn-dra
        template:
          metadata:
            labels:
              app: neuron-burn-dra
              neuron-test: "true"
              ci-test.example.com/pod-color: violet
          spec:
            nodeSelector:
              node.kubernetes.io/instance-type: ${var.instance_type}
            resourceClaims:
            - name: neuron
              resourceClaimTemplateName: neuron-1
            containers:
            - name: neuron-burn
              image: public.ecr.aws/neuron/pytorch-inference-neuronx:2.1.2-neuronx-py310-sdk2.20.2-ubuntu20.04
              command: ["python3", "-c"]
              args:
              - |
                import torch, torch_neuronx, time
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
                claims:
                - name: neuron
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
    EOT
  }
}

# --- Wait for neuron-monitor pods ---

resource "null_resource" "wait_neuron_monitor" {
  depends_on = [null_resource.restart_pods, null_resource.neuron_burn_dra]
  triggers   = { timestamp = timestamp() }
  provisioner "local-exec" {
    command = <<-EOT
      echo "Waiting for neuron-monitor pods to be ready..."
      for i in $(seq 1 30); do
        READY=$(kubectl -n amazon-cloudwatch get pods -l app.kubernetes.io/name=neuron-monitor --no-headers 2>/dev/null | grep -c "Running" || true)
        if [ "$READY" -ge 1 ]; then
          echo "neuron-monitor ready ($READY running)"
          break
        fi
        echo "Attempt $i: $READY neuron-monitor pods running, waiting..."
        sleep 10
      done
    EOT
  }
}

# --- Test runner ---

resource "null_resource" "validator" {
  depends_on = [null_resource.wait_neuron_monitor]
  triggers   = { always_run = timestamp() }
  provisioner "local-exec" {
    command = <<-EOT
      echo "Running OTEL Neuron DRA cluster integration tests"
      cd ../../../..

      echo "Waiting for Neuron runtime to initialize (image pull + trace compile)..."
      READY=0
      for i in $(seq 1 90); do
        READY=$(kubectl logs -n default -l app=neuron-burn-dra --tail=5 2>/dev/null | grep -c "^Iteration " || true)
        if [ "$READY" -gt 0 ]; then
          echo "Neuron burn loop active (after $((i*10))s)"
          break
        fi
        if [ $((i % 6)) -eq 0 ]; then
          echo "--- Attempt $i ($((i*10))s): iter_lines=$READY ---"
          kubectl get pods -n default -l app=neuron-burn-dra -o wide 2>&1 | head || true
        fi
        sleep 10
      done
      if [ "$READY" -eq 0 ]; then
        echo "ERROR: neuron-burn-dra loop not active after 15 minutes"
        kubectl get pods -n default -l app=neuron-burn-dra -o wide 2>&1 || true
        kubectl describe pod -n default -l app=neuron-burn-dra 2>&1 | tail -40 || true
        kubectl get resourceclaims -n default 2>&1 || true
        exit 1
      fi

      echo "Waiting 6 minutes for metrics to propagate (Zeus 5-min staleness window)..."
      sleep 360

      go test -tags integration -timeout 1h -v ${var.test_dir} \
        -eksClusterName=${aws_eks_cluster.this.name} \
        -computeType=EKS \
        -eksDeploymentStrategy=DAEMON \
        -region=${var.region}
    EOT
  }
}
