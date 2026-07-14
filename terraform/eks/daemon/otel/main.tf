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

# EKS Node Group — 2x t3.medium with node-color=blue label
resource "aws_eks_node_group" "this" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "cwagent-otel-integ-node-${module.common.testing_id}"
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

  labels = {
    "ci-test.example.com/node-color" = "blue"
  }

  depends_on = [
    aws_iam_role_policy_attachment.node_AmazonEC2ContainerRegistryReadOnly,
    aws_iam_role_policy_attachment.node_AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy,
  ]
}

# EKS Node IAM Role
resource "aws_iam_role" "node_role" {
  name = "cwagent-otel-eks-Worker-Role-${module.common.testing_id}"
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
  name = "cwagent-otel-pod-identity-${module.common.testing_id}"
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

# Pre-create the events log group and stream — the OTLP HTTP exporter's
# x-aws-log-group-create header is not yet honored by the backend.
resource "aws_cloudwatch_log_group" "events" {
  name              = "/aws/containerinsights/${aws_eks_cluster.this.name}/events"
  retention_in_days = 1
}

resource "aws_cloudwatch_log_stream" "events" {
  name           = "events"
  log_group_name = aws_cloudwatch_log_group.events.name
}

resource "helm_release" "aws_observability" {
  name             = "amazon-cloudwatch-observability"
  chart            = "./helm-charts/charts/amazon-cloudwatch-observability"
  namespace        = "amazon-cloudwatch"
  create_namespace = true

  set = [
    { name = "clusterName", value = aws_eks_cluster.this.name },
    { name = "region", value = var.region },
    { name = "otelContainerInsights.events.enabled", value = "true" }
  ]

  depends_on = [
    aws_eks_addon.pod_identity_agent,
    null_resource.kubectl,
    data.external.clone_helm_chart,
    aws_cloudwatch_log_stream.events,
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

# --- Test workload: nginx ---

resource "kubernetes_deployment_v1" "nginx_test" {
  depends_on = [aws_eks_node_group.this]
  metadata {
    name      = "nginx-test"
    namespace = "default"
  }
  spec {
    replicas = 1
    selector { match_labels = { app = "nginx-test" } }
    template {
      metadata { labels = { app = "nginx-test" } }
      spec {
        container {
          name  = "nginx"
          image = "public.ecr.aws/nginx/nginx:latest"
          port { container_port = 80 }
        }
      }
    }
  }
}

# --- KSM test workloads ---

resource "kubernetes_stateful_set_v1" "ksm_statefulset" {
  depends_on = [aws_eks_node_group.this]
  metadata {
    name      = "ksm-test-statefulset"
    namespace = "default"
  }
  spec {
    replicas     = 1
    service_name = kubernetes_service_v1.ksm_statefulset_headless.metadata[0].name
    selector { match_labels = { app = "ksm-test-statefulset" } }
    template {
      metadata { labels = { app = "ksm-test-statefulset" } }
      spec {
        node_selector = { "ci-test.example.com/node-color" = "blue" }
        container {
          name  = "pause"
          image = "registry.k8s.io/pause:3.9"
          resources { requests = { cpu = "10m", memory = "16Mi" } }
        }
      }
    }
  }
}

resource "kubernetes_service_v1" "ksm_statefulset_headless" {
  metadata {
    name      = "ksm-test-statefulset"
    namespace = "default"
  }
  spec {
    cluster_ip = "None"
    selector   = { app = "ksm-test-statefulset" }
    port {
      port = 80
      name = "placeholder"
    }
  }
}

resource "kubernetes_cron_job_v1" "ksm_cronjob" {
  depends_on = [aws_eks_node_group.this]
  metadata {
    name      = "ksm-test-cronjob"
    namespace = "default"
  }
  spec {
    schedule                      = "*/5 * * * *"
    successful_jobs_history_limit = 1
    failed_jobs_history_limit     = 1
    job_template {
      metadata {}
      spec {
        template {
          metadata { labels = { app = "ksm-test-cronjob" } }
          spec {
            node_selector  = { "ci-test.example.com/node-color" = "blue" }
            restart_policy = "Never"
            container {
              name    = "echo"
              image   = "busybox:1.36"
              command = ["echo", "ksm-test"]
              resources { requests = { cpu = "10m", memory = "16Mi" } }
            }
          }
        }
      }
    }
  }
}

resource "kubernetes_job_v1" "ksm_job" {
  depends_on = [aws_eks_node_group.this]
  metadata {
    name      = "ksm-test-job"
    namespace = "default"
  }
  spec {
    ttl_seconds_after_finished = 86400
    template {
      metadata { labels = { app = "ksm-test-job" } }
      spec {
        node_selector  = { "ci-test.example.com/node-color" = "blue" }
        restart_policy = "Never"
        container {
          name    = "echo"
          image   = "busybox:1.36"
          command = ["echo", "ksm-test"]
          resources { requests = { cpu = "10m", memory = "16Mi" } }
        }
      }
    }
  }
}

resource "null_resource" "ksm_replicaset" {
  depends_on = [aws_eks_node_group.this, null_resource.kubectl]
  provisioner "local-exec" {
    command = <<-EOT
      cat <<'EOF' | kubectl apply -f -
      apiVersion: apps/v1
      kind: ReplicaSet
      metadata:
        name: ksm-test-replicaset
        namespace: default
      spec:
        replicas: 1
        selector:
          matchLabels:
            app: ksm-test-replicaset
        template:
          metadata:
            labels:
              app: ksm-test-replicaset
          spec:
            nodeSelector:
              ci-test.example.com/node-color: blue
            containers:
            - name: pause
              image: registry.k8s.io/pause:3.9
              resources:
                requests:
                  cpu: 10m
                  memory: 16Mi
      EOF
    EOT
  }
}

# --- Test runner ---

resource "null_resource" "validator" {
  depends_on = [
    null_resource.restart_pods,
    kubernetes_deployment_v1.nginx_test,
  ]

  triggers = { always_run = timestamp() }

  provisioner "local-exec" {
    command = <<-EOT
      echo "Running OTEL standard cluster integration tests"
      cd ../../../..

      echo "Waiting 5 minutes for metrics and events to propagate..."
      sleep 300

      go test -tags integration -timeout 1h -v ${var.test_dir} \
        -eksClusterName=${aws_eks_cluster.this.name} \
        -computeType=EKS \
        -eksDeploymentStrategy=DAEMON \
        -region=${var.region}
    EOT
  }
}
