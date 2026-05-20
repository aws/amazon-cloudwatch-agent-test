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

  # Sentinel labels planted on nodes as canaries for tier-dropping tests.
  tier1_sentinels = { "helm.sh/chart" = "test-chart", "release" = "test-release" }
  tier2_sentinels = { "pod-template-generation" = "1" }
  tier3_sentinels = { "karpenter.sh/sentinel-known" = "v", "nvidia.com/sentinel-known" = "v" }
  tier6_sentinels = { "ci-test.example.com/sentinel-customer-a" = "v", "ci-test.example.com/sentinel-customer-b" = "v" }

  # Padding labels to inflate attribute count.
  pad_20  = { for i in range(1, 21) : "ci-test.example.com/pad-${format("%03d", i)}" => "v" }
  pad_108 = { for i in range(1, 109) : "ci-test.example.com/pad-${format("%03d", i)}" => "v" }
  pad_120 = { for i in range(1, 121) : "ci-test.example.com/pad-${format("%03d", i)}" => "v" }

  common_sentinels = merge(local.tier1_sentinels, local.tier2_sentinels, local.tier3_sentinels, local.tier6_sentinels)

  # Pod padding labels for the high nginx deployment (135 labels as YAML lines).
  # 135 padding + 3 explicit (app, app.kubernetes.io/part-of, aaa-sentinel-pod) = 138 pod labels.
  # After all tier 1-6 node labels are dropped, the processor enters tier 7/8.
  # The sentinel is named "aaa-sentinel-pod" to sort alphabetically BEFORE the
  # pod-pad-* labels, ensuring it is dropped first when the processor prunes tier 8.
  high_pod_padding_yaml = join("\n", [for i in range(1, 136) : "        ci-test.example.com/pod-pad-${format("%03d", i)}: v"])

  low_labels = merge(local.common_sentinels, local.pad_108, {
    "ci-test.example.com/node-color"      = "white"
    "ci-test.example.com/attr-limit-node" = "node-low"
  })

  mid_labels = merge(local.common_sentinels, local.pad_120, {
    "ci-test.example.com/node-color"      = "white"
    "ci-test.example.com/attr-limit-node" = "node-mid"
  })

  high_labels = merge(local.common_sentinels, local.pad_20, {
    "ci-test.example.com/node-color"      = "white"
    "ci-test.example.com/attr-limit-node" = "node-high"
  })
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

# --- 3 Node Groups: low, mid, high ---

resource "aws_eks_node_group" "low" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "cwagent-attr-limit-low-${module.common.testing_id}"
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
  labels         = local.low_labels

  depends_on = [
    aws_iam_role_policy_attachment.node_AmazonEC2ContainerRegistryReadOnly,
    aws_iam_role_policy_attachment.node_AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy,
  ]
}

resource "aws_eks_node_group" "mid" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "cwagent-attr-limit-mid-${module.common.testing_id}"
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
  labels         = local.mid_labels

  depends_on = [
    aws_iam_role_policy_attachment.node_AmazonEC2ContainerRegistryReadOnly,
    aws_iam_role_policy_attachment.node_AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy,
  ]
}

resource "aws_eks_node_group" "high" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "cwagent-attr-limit-high-${module.common.testing_id}"
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
  labels         = local.high_labels

  depends_on = [
    aws_iam_role_policy_attachment.node_AmazonEC2ContainerRegistryReadOnly,
    aws_iam_role_policy_attachment.node_AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy,
  ]
}

# EKS Node IAM Role
resource "aws_iam_role" "node_role" {
  name = "cwagent-attr-limit-eks-Worker-Role-${module.common.testing_id}"
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
  name = "cwagent-attr-limit-pod-identity-${module.common.testing_id}"
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
  depends_on   = [aws_eks_node_group.low, aws_eks_node_group.mid, aws_eks_node_group.high]
  cluster_name = aws_eks_cluster.this.name
  addon_name   = "eks-pod-identity-agent"
}

# --- Update kubeconfig ---

resource "null_resource" "kubectl" {
  depends_on = [aws_eks_cluster.this, aws_eks_node_group.low, aws_eks_node_group.mid, aws_eks_node_group.high]
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

# --- Test workloads: 3 nginx deployments via kubectl apply ---

resource "null_resource" "nginx_low" {
  depends_on = [aws_eks_node_group.low, null_resource.kubectl]
  provisioner "local-exec" {
    command = <<-EOT
      cat <<'EOF' | kubectl apply -f -
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: attr-limit-nginx-low
        namespace: default
      spec:
        replicas: 1
        selector:
          matchLabels:
            app: attr-limit-nginx-low
        template:
          metadata:
            labels:
              app: attr-limit-nginx-low
          spec:
            nodeSelector:
              ci-test.example.com/attr-limit-node: node-low
            containers:
            - name: nginx
              image: public.ecr.aws/nginx/nginx:latest
              ports:
              - containerPort: 80
      EOF
    EOT
  }
}

resource "null_resource" "nginx_mid" {
  depends_on = [aws_eks_node_group.mid, null_resource.kubectl]
  provisioner "local-exec" {
    command = <<-EOT
      cat <<'EOF' | kubectl apply -f -
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: attr-limit-nginx-mid
        namespace: default
      spec:
        replicas: 1
        selector:
          matchLabels:
            app: attr-limit-nginx-mid
        template:
          metadata:
            labels:
              app: attr-limit-nginx-mid
          spec:
            nodeSelector:
              ci-test.example.com/attr-limit-node: node-mid
            containers:
            - name: nginx
              image: public.ecr.aws/nginx/nginx:latest
              ports:
              - containerPort: 80
      EOF
    EOT
  }
}

resource "null_resource" "nginx_high" {
  depends_on = [aws_eks_node_group.high, null_resource.kubectl]
  provisioner "local-exec" {
    command = <<-EOT
      cat <<EOF | kubectl apply -f -
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: attr-limit-nginx-high
        namespace: default
      spec:
        replicas: 1
        selector:
          matchLabels:
            app: attr-limit-nginx-high
        template:
          metadata:
            labels:
              app: attr-limit-nginx-high
              app.kubernetes.io/part-of: attr-limit-test
              ci-test.example.com/aaa-sentinel-pod: v
      ${local.high_pod_padding_yaml}
          spec:
            nodeSelector:
              ci-test.example.com/attr-limit-node: node-high
            containers:
            - name: nginx
              image: public.ecr.aws/nginx/nginx:latest
              ports:
              - containerPort: 80
      EOF
    EOT
  }
}

# --- Test runner ---

resource "null_resource" "validator" {
  depends_on = [
    null_resource.restart_pods,
    null_resource.nginx_low,
    null_resource.nginx_mid,
    null_resource.nginx_high,
  ]

  triggers = { always_run = timestamp() }

  provisioner "local-exec" {
    command = <<-EOT
      echo "Running OTEL attr_limit cluster integration tests"
      cd ../../../..

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
