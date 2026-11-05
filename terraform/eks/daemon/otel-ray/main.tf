# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
# SPDX-License-Identifier: MIT

# OTEL Container Insights Ray solution.
#
# Builds a CPU-only cluster with the KubeRay operator and a RayService running a
# one-deployment Ray Serve app (one head, one worker), shipped in a ConfigMap. The pod templates declare no ports,
# so KubeRay adds its default metrics:8080, the port the chart's ray job scrapes.
# A load generator sends requests to the Serve app, then the chart is installed
# with otelContainerInsights enabled (solutions default on) and
# ./test/otel/solutions/ray runs.

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
  aws_eks   = "aws eks --region ${var.region}"
  namespace = "ray"
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
  node_group_name = "cwagent-otel-ray-integ-node-${module.common.testing_id}"
  node_role_arn   = aws_iam_role.node_role.arn
  subnet_ids      = module.basic_components.public_subnet_ids

  scaling_config {
    desired_size = 2
    max_size     = 2
    min_size     = 2
  }

  ami_type      = var.ami_type
  capacity_type = "ON_DEMAND"
  # The Ray image is a few GB.
  disk_size      = 40
  instance_types = [var.instance_type]

  depends_on = [
    aws_iam_role_policy_attachment.node_AmazonEC2ContainerRegistryReadOnly,
    aws_iam_role_policy_attachment.node_AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy,
  ]
}

resource "aws_iam_role" "node_role" {
  name = "cwagent-otel-ray-Worker-Role-${module.common.testing_id}"
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
  name = "cwagent-otel-ray-pod-identity-${module.common.testing_id}"
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

resource "aws_eks_addon" "pod_identity_agent" {
  depends_on   = [aws_eks_node_group.this]
  cluster_name = aws_eks_cluster.this.name
  addon_name   = "eks-pod-identity-agent"
}

resource "null_resource" "kubectl" {
  depends_on = [aws_eks_cluster.this, aws_eks_node_group.this]
  provisioner "local-exec" {
    command = "${local.aws_eks} update-kubeconfig --name ${aws_eks_cluster.this.name}"
  }
}

# --- KubeRay and the Ray Serve workload ---

resource "helm_release" "kuberay_operator" {
  name             = "kuberay-operator"
  repository       = "https://ray-project.github.io/kuberay-helm/"
  chart            = "kuberay-operator"
  version          = var.kuberay_version
  namespace        = "kuberay"
  create_namespace = true
  wait             = true
  depends_on       = [null_resource.kubectl]
}

resource "kubernetes_namespace_v1" "ray" {
  depends_on = [aws_eks_node_group.this]
  metadata { name = local.namespace }
}

# A one-deployment Ray Serve app, mounted into the Ray pods so the test does not
# download code at run time.
resource "kubernetes_config_map_v1" "serve_app" {
  depends_on = [kubernetes_namespace_v1.ray]
  metadata {
    name      = "serve-app"
    namespace = local.namespace
  }
  data = {
    "fruit_app.py" = <<-EOT
      from ray import serve
      from starlette.requests import Request


      @serve.deployment(ray_actor_options={"num_cpus": 0.1})
      class FruitMarket:
          def __init__(self, price: float = 3.0):
              self.price = price

          async def __call__(self, request: Request) -> dict:
              fruit, amount = await request.json()
              return {"fruit": fruit, "total": self.price * amount}


      app = FruitMarket.bind()
    EOT
  }
}

# A CPU Ray Serve app. Neither group declares ports: KubeRay adds its defaults,
# including metrics:8080.
resource "null_resource" "ray_service" {
  depends_on = [helm_release.kuberay_operator, kubernetes_config_map_v1.serve_app]
  provisioner "local-exec" {
    command = <<-EOT
      set -e
      cat <<'EOF' | kubectl apply -f -
      apiVersion: ray.io/v1
      kind: RayService
      metadata:
        name: fruit
        namespace: ${local.namespace}
      spec:
        serveConfigV2: |
          applications:
            - name: fruit_app
              import_path: fruit_app:app
              route_prefix: /fruit
        rayClusterConfig:
          rayVersion: "${var.ray_version}"
          headGroupSpec:
            # No CPUs for tasks on the head, so the Serve replica runs on the worker,
            # whose readiness probe needs a Serve proxy, as KubeRay recommends.
            rayStartParams: {dashboard-host: "0.0.0.0", num-cpus: "0"}
            template:
              spec:
                volumes: [{name: serve-app, configMap: {name: serve-app}}]
                containers:
                  - name: ray-head
                    image: ${var.ray_image}
                    env: [{name: PYTHONPATH, value: /home/ray/app}]
                    volumeMounts: [{name: serve-app, mountPath: /home/ray/app}]
                    resources:
                      requests: {cpu: "1", memory: 3Gi}
                      limits: {memory: 4Gi}
          workerGroupSpecs:
            - groupName: cpu
              replicas: 1
              minReplicas: 1
              maxReplicas: 1
              rayStartParams: {}
              template:
                spec:
                  volumes: [{name: serve-app, configMap: {name: serve-app}}]
                  containers:
                    - name: ray-worker
                      image: ${var.ray_image}
                      env: [{name: PYTHONPATH, value: /home/ray/app}]
                      volumeMounts: [{name: serve-app, mountPath: /home/ray/app}]
                      resources:
                        requests: {cpu: "1", memory: 3Gi}
                        limits: {memory: 4Gi}
      EOF
      for i in $(seq 1 90); do
        s=$(kubectl -n ${local.namespace} get rayservice fruit -o jsonpath='{.status.serviceStatus}' 2>/dev/null || true)
        [ "$s" = "Running" ] && exit 0
        sleep 20
      done
      echo "RayService fruit did not reach Running" >&2
      kubectl -n ${local.namespace} get rayservice fruit -o yaml >&2 || true
      exit 1
    EOT
  }
  provisioner "local-exec" {
    when    = destroy
    command = "kubectl -n ray delete rayservice fruit --timeout=120s 2>/dev/null || true"
  }
}

resource "kubernetes_deployment_v1" "loadgen" {
  depends_on = [null_resource.ray_service]
  metadata {
    name      = "loadgen"
    namespace = local.namespace
  }
  spec {
    replicas = 1
    selector { match_labels = { app = "loadgen" } }
    template {
      metadata { labels = { app = "loadgen" } }
      spec {
        container {
          name  = "curl"
          image = "curlimages/curl:8.11.1"
          command = ["sh", "-c", <<-EOT
            while true; do
              curl -s -o /dev/null -m 10 -X POST -H 'Content-Type: application/json' -d '["MANGO", 2]' \
                http://fruit-serve-svc.${local.namespace}.svc.cluster.local:8000/fruit
              sleep 2
            done
          EOT
          ]
          resources { requests = { cpu = "10m", memory = "16Mi" } }
        }
      }
    }
  }
}

# --- Helm chart install ---

data "external" "clone_helm_chart" {
  program = ["bash", "-c", <<-EOT
    set -e
    rm -rf ./helm-charts
    git clone -b ${var.helm_chart_branch} ${var.helm_chart_repo} ./helm-charts >&2
    # A chart without the Ray solution would install fine and fail every test;
    # stop here with the reason instead.
    values=./helm-charts/charts/amazon-cloudwatch-observability/values.yaml
    if ! grep -qE "^ +ray:" "$values"; then
      echo "${var.helm_chart_repo}@${var.helm_chart_branch} has no 'ray' solution in values.yaml" >&2
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

  depends_on = [
    aws_eks_addon.pod_identity_agent,
    null_resource.kubectl,
    data.external.clone_helm_chart,
  ]
}

resource "aws_eks_pod_identity_association" "cloudwatch_agent" {
  depends_on      = [helm_release.aws_observability]
  cluster_name    = aws_eks_cluster.this.name
  namespace       = "amazon-cloudwatch"
  service_account = "cloudwatch-agent"
  role_arn        = aws_iam_role.pod_identity_role.arn
}

resource "null_resource" "update_image" {
  depends_on = [helm_release.aws_observability, null_resource.kubectl]
  triggers   = { timestamp = timestamp() }
  provisioner "local-exec" {
    command = <<-EOT
      set -e
      sleep 30
      kubectl -n amazon-cloudwatch patch AmazonCloudWatchAgent cloudwatch-agent --type='json' \
        -p='[{"op": "replace", "path": "/spec/image", "value": "${var.cwagent_image_repo}:${var.cwagent_image_tag}"}]'
      sleep 10
    EOT
  }
}

resource "null_resource" "restart_pods" {
  depends_on = [aws_eks_pod_identity_association.cloudwatch_agent, null_resource.update_image]
  triggers   = { timestamp = timestamp() }
  provisioner "local-exec" {
    command = <<-EOT
      set -e
      kubectl -n amazon-cloudwatch rollout restart daemonset/cloudwatch-agent
      kubectl -n amazon-cloudwatch rollout status daemonset/cloudwatch-agent --timeout=180s
    EOT
  }
}

# --- Test runner ---

resource "null_resource" "validator" {
  depends_on = [
    null_resource.restart_pods,
    kubernetes_deployment_v1.loadgen,
  ]

  triggers = { always_run = timestamp() }

  provisioner "local-exec" {
    command = <<-EOT
      set -e
      cd ../../../..
      echo "Waiting 4 minutes for metrics to propagate..."
      sleep 240
      echo "Running OTEL solutions tests (${var.test_dir})..."
      go test -tags integration -timeout 1h -v ${var.test_dir}/... \
        -eksClusterName=${aws_eks_cluster.this.name} \
        -computeType=EKS \
        -eksDeploymentStrategy=DAEMON \
        -region=${var.region}
    EOT
  }
}
